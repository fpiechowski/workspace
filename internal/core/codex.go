package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Codex app-server uses newline-delimited JSON-RPC over stdio. This bridge keeps
// the process in the tmux pane, renders its output and queues inputs only between
// turns. It never injects keystrokes into another application's terminal.
type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
type pendingRPC struct {
	Kind     string
	Messages []string
}

func (s *Service) runCodex(ctx context.Context, selector string, session Session, cmd *exec.Cmd, userInput io.Reader, out, errOut io.Writer) error {
	cmd.Stdin = nil
	cmd.Stdout = nil
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan rpcMessage, 128)
	readErrors := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 16*1024*1024)
		for scanner.Scan() {
			var message rpcMessage
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				select {
				case readErrors <- err:
				case <-ctx.Done():
				}
				return
			}
			select {
			case events <- message:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case readErrors <- err:
		case <-ctx.Done():
		}
	}()
	lines := make(chan string, 16)
	if userInput != nil {
		go func() {
			scanner := bufio.NewScanner(userInput)
			scanner.Buffer(make([]byte, 4096), 1024*1024)
			for scanner.Scan() {
				select {
				case lines <- scanner.Text():
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	encoder := json.NewEncoder(stdin)
	counter := 0
	pending := map[int]pendingRPC{}
	send := func(method string, params any, kind string, ids []string) error {
		counter++
		pending[counter] = pendingRPC{kind, ids}
		return encoder.Encode(map[string]any{"id": counter, "method": method, "params": params})
	}
	if err := send("initialize", map[string]any{"clientInfo": map[string]string{"name": "workspace_cli", "title": "workspace", "version": "0.1.0"}}, "initialize", nil); err != nil {
		return err
	}
	thread := session.ClientThreadID
	busy := true
	ready := false
	turnID := ""
	requests := map[string]rpcMessage{}
	bootstrap, err := os.ReadFile(session.PromptFile)
	if err != nil {
		return err
	}
	startTurn := func(text string, ids []string) error {
		busy = true
		if err := s.clientState(ctx, selector, session.ID, "", "busy"); err != nil {
			return err
		}
		return send("turn/start", map[string]any{"threadId": thread, "input": []any{map[string]any{"type": "text", "text": text}}}, "turn", ids)
	}
	fmt.Fprintf(out, "workspace · %s · %s\nType a message; /quit closes this session. /help lists controls.\n", session.AgentSnapshot.Name, session.Route.Model)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErrors:
			return fmt.Errorf("codex app-server stopped: %w", err)
		case message := <-events:
			if len(message.ID) > 0 && message.Method == "" {
				var id int
				if err := json.Unmarshal(message.ID, &id); err != nil {
					continue
				}
				request, ok := pending[id]
				if !ok {
					continue
				}
				delete(pending, id)
				if message.Error != nil {
					if request.Kind == "initialize" || request.Kind == "thread" {
						return fail("client_error", "%s", message.Error.Message)
					}
					busy = false
					_ = s.clientState(ctx, selector, session.ID, "", "idle")
					fmt.Fprintln(errOut, "Codex:", message.Error.Message)
					continue
				}
				switch request.Kind {
				case "initialize":
					if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
						return err
					}
					params := map[string]any{}
					for k, v := range session.ClientSnapshot.ThreadParams {
						params[k] = v
					}
					params["cwd"] = session.CWD
					params["model"] = session.Route.Model
					method := "thread/start"
					if thread != "" {
						method = "thread/resume"
						params["threadId"] = thread
					}
					if err := send(method, params, "thread", nil); err != nil {
						return err
					}
				case "thread":
					var response struct {
						Thread struct {
							ID string `json:"id"`
						} `json:"thread"`
					}
					if err := json.Unmarshal(message.Result, &response); err != nil {
						return err
					}
					if response.Thread.ID == "" {
						return fail("client_error", "Codex did not return a thread ID")
					}
					thread = response.Thread.ID
					ready = true
					if err := s.clientState(ctx, selector, session.ID, thread, "idle"); err != nil {
						return err
					}
					if err := startTurn(string(bootstrap), nil); err != nil {
						return err
					}
				case "turn":
					var response struct {
						Turn struct {
							ID string `json:"id"`
						} `json:"turn"`
					}
					_ = json.Unmarshal(message.Result, &response)
					turnID = response.Turn.ID
					if len(request.Messages) > 0 {
						if err := s.markDelivered(ctx, selector, session.ID, request.Messages); err != nil {
							return err
						}
					}
				}
				continue
			}
			if len(message.ID) > 0 && message.Method != "" {
				key := string(message.ID)
				requests[key] = message
				showNativeRequest(out, key, message)
				if err := s.clientState(ctx, selector, session.ID, "", "needs_input"); err != nil {
					return err
				}
				continue
			}
			switch message.Method {
			case "item/agentMessage/delta":
				var p struct {
					Delta string `json:"delta"`
				}
				_ = json.Unmarshal(message.Params, &p)
				fmt.Fprint(out, p.Delta)
			case "item/commandExecution/outputDelta":
				var p struct {
					Delta string `json:"delta"`
				}
				_ = json.Unmarshal(message.Params, &p)
				fmt.Fprint(out, p.Delta)
			case "item/started":
				var p struct {
					Item struct{ Type, Command string } `json:"item"`
				}
				_ = json.Unmarshal(message.Params, &p)
				if p.Item.Command != "" {
					fmt.Fprintln(out, "\n$", p.Item.Command)
				}
			case "turn/completed":
				busy = false
				turnID = ""
				requests = map[string]rpcMessage{}
				var p struct {
					Turn struct {
						Status string `json:"status"`
						Error  any    `json:"error"`
					} `json:"turn"`
				}
				_ = json.Unmarshal(message.Params, &p)
				if p.Turn.Status == "failed" {
					fmt.Fprintf(errOut, "\nTurn failed: %v\n", p.Turn.Error)
				}
				fmt.Fprintln(out, "\n[ready]")
				if err := s.clientState(ctx, selector, session.ID, "", "idle"); err != nil {
					return err
				}
			}
		case line := <-lines:
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if line == "/quit" {
				return nil
			}
			if line == "/help" {
				fmt.Fprintln(out, "Send text to converse; /interrupt cancels the active turn; /approve ID or /decline ID answers an approval; /respond ID JSON answers another request; /quit closes the session.")
				continue
			}
			if line == "/interrupt" {
				if turnID != "" {
					if err := send("turn/interrupt", map[string]string{"threadId": thread, "turnId": turnID}, "interrupt", nil); err != nil {
						return err
					}
				}
				continue
			}
			fields := strings.SplitN(line, " ", 3)
			if len(fields) >= 2 && (fields[0] == "/approve" || fields[0] == "/decline" || fields[0] == "/respond" || fields[0] == "/answer") {
				request, ok := requests[fields[1]]
				if !ok {
					fmt.Fprintln(errOut, "Unknown request ID")
					continue
				}
				var result any
				if fields[0] == "/answer" {
					if len(fields) != 3 {
						fmt.Fprintln(errOut, "Provide an answer after the request ID")
						continue
					}
					var err error
					result, err = nativeAnswer(request, fields[2])
					if err != nil {
						fmt.Fprintln(errOut, err)
						continue
					}
				} else if fields[0] == "/respond" {
					if len(fields) != 3 || json.Unmarshal([]byte(fields[2]), &result) != nil {
						fmt.Fprintln(errOut, "Provide a JSON result")
						continue
					}
				} else {
					if !strings.HasSuffix(request.Method, "requestApproval") {
						fmt.Fprintln(errOut, "Use /respond for this non-approval request")
						continue
					}
					decision := "decline"
					if fields[0] == "/approve" {
						decision = "accept"
					}
					result = map[string]string{"decision": decision}
				}
				if err := encoder.Encode(map[string]any{"id": request.ID, "result": result}); err != nil {
					return err
				}
				delete(requests, fields[1])
				continue
			}
			user := *s
			user.Actor = Actor{}
			if _, err := user.SendMessage(ctx, selector, MessageOptions{To: session.AgentID, Body: line}); err != nil {
				fmt.Fprintln(errOut, err)
			}
		case <-ticker.C:
			if !ready || busy || len(requests) > 0 {
				continue
			}
			var messages []Message
			exit := false
			err := s.With(ctx, selector, func(d *Document) error {
				p, err := findSession(d, session.ID)
				if err != nil {
					return err
				}
				if !p.Active() {
					return fail("stale_session", "session was stopped")
				}
				if d.State.Status == "completed" || d.State.Status == "archived" {
					exit = true
					return nil
				}
				if p.TaskID != "" {
					t, err := findTask(d, p.TaskID)
					if err != nil {
						return err
					}
					if t.State == "accepted" || t.Attempt != p.TaskAttempt {
						exit = true
						return nil
					}
				}
				for _, m := range d.Registry.Messages {
					if m.ToAgent == p.AgentID && m.AcknowledgedAt == nil && m.DeliveredSessionID != p.ID {
						messages = append(messages, m)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			if exit {
				return nil
			}
			if len(messages) == 0 {
				continue
			}
			var text strings.Builder
			ids := []string{}
			for _, m := range messages {
				text.WriteString("Message " + m.ID + " from " + m.FromAgent + " (" + m.Kind + "):\n" + m.Body + "\n")
				if m.HandoffID != "" {
					text.WriteString("Read handoff " + m.HandoffID + " and its artifacts before deciding.\n")
				}
				ids = append(ids, m.ID)
			}
			text.WriteString("Acknowledge each handled message with workspace inbox ack <id>. ACK does not accept a task result.\n")
			if err := startTurn(text.String(), ids); err != nil {
				return err
			}
		}
	}
}

// References used to validate the stable transport and lifecycle contract:
// https://learn.chatgpt.com/docs/app-server
