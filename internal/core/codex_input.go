package core

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type nativeQuestion struct {
	ID       string                                `json:"id"`
	Question string                                `json:"question"`
	Options  []struct{ Label, Description string } `json:"options"`
}

func nativeQuestions(request rpcMessage) []nativeQuestion {
	var p struct {
		Questions []nativeQuestion `json:"questions"`
	}
	_ = json.Unmarshal(request.Params, &p)
	return p.Questions
}
func showNativeRequest(out io.Writer, key string, request rpcMessage) {
	if strings.HasSuffix(request.Method, "requestApproval") {
		var p struct{ Command, Cwd, Reason, GrantRoot string }
		_ = json.Unmarshal(request.Params, &p)
		fmt.Fprintf(out, "\nApproval required [%s]\n", key)
		if p.Command != "" {
			fmt.Fprintln(out, "Command:", p.Command)
		}
		if p.Cwd != "" {
			fmt.Fprintln(out, "Directory:", p.Cwd)
		}
		if p.Reason != "" {
			fmt.Fprintln(out, p.Reason)
		}
		if p.GrantRoot != "" {
			fmt.Fprintln(out, "Requested file access:", p.GrantRoot)
		}
		if p.Command == "" && p.Reason == "" && p.GrantRoot == "" {
			fmt.Fprintln(out, string(request.Params))
		}
		fmt.Fprintf(out, "/approve %s  |  /decline %s\n", key, key)
		return
	}
	questions := nativeQuestions(request)
	if len(questions) > 0 {
		for _, q := range questions {
			fmt.Fprintf(out, "\n%s\n", q.Question)
			for n, opt := range q.Options {
				fmt.Fprintf(out, "  %d. %s — %s\n", n+1, opt.Label, opt.Description)
			}
		}
		if len(questions) == 1 {
			fmt.Fprintf(out, "Reply: /answer %s <your answer or option label>\n", key)
			return
		}
	}
	fmt.Fprintf(out, "\nInput required [%s]: %s\n%s\nReply: /respond %s <JSON result>\n", key, request.Method, string(request.Params), key)
}
func nativeAnswer(request rpcMessage, text string) (any, error) {
	questions := nativeQuestions(request)
	if request.Method != "item/tool/requestUserInput" || len(questions) != 1 {
		return nil, fail("invalid_answer", "use /respond for this request")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fail("invalid_answer", "answer cannot be empty")
	}
	return map[string]any{"answers": map[string]any{questions[0].ID: map[string]any{"answers": []string{text}}}}, nil
}
