package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type openCodeMockRequest struct {
	Method string
	Path   string
	Body   string
}

type openCodeMock struct {
	server *httptest.Server
	thread string
	marker string

	mu                   sync.Mutex
	requests             []openCodeMockRequest
	historyMarker        bool
	markerOnSubmit       bool
	historyExtra         []string
	historyExtraOnSubmit []string
	selectFalse          bool
	appendFalse          bool
	appendStatus         int
	appendBody           string
	submitFalse          bool
	submitStatus         int
	submitBody           string
	appendBlock          bool
	appendBlockOnce      bool
	submitBlock          bool
	onSubmit             func()
}

type fakeNotificationRuntime struct {
	*fakeRuntime
	mu       sync.Mutex
	displays []string
}

func (r *fakeNotificationRuntime) DisplayMessage(_ context.Context, paneID, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.displays = append(r.displays, paneID+": "+message)
	return nil
}

func (r *fakeNotificationRuntime) displayCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.displays)
}

func newOpenCodeMock(t *testing.T, thread, marker string) *openCodeMock {
	t.Helper()
	m := &openCodeMock{thread: thread, marker: marker, appendStatus: http.StatusOK, appendBody: "true", submitStatus: http.StatusOK, submitBody: "true"}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.requests = append(m.requests, openCodeMockRequest{Method: r.Method, Path: r.URL.RequestURI(), Body: string(body)})
		m.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/global/health":
			writeOpenCodeMockResponse(w, http.StatusOK, "{}")
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/message"):
			m.mu.Lock()
			marker := m.historyMarker
			extra := append([]string(nil), m.historyExtra...)
			m.mu.Unlock()
			if marker {
				markers := append([]string{m.marker}, extra...)
				body, _ := json.Marshal(markers)
				writeOpenCodeMockResponse(w, http.StatusOK, string(body))
			} else {
				writeOpenCodeMockResponse(w, http.StatusOK, "[]")
			}
		case r.Method == http.MethodPost && r.URL.Path == "/tui/select-session":
			m.mu.Lock()
			falseResponse := m.selectFalse
			m.mu.Unlock()
			if falseResponse {
				writeOpenCodeMockResponse(w, http.StatusOK, "false")
			} else {
				writeOpenCodeMockResponse(w, http.StatusOK, "true")
			}
		case r.Method == http.MethodPost && r.URL.Path == "/tui/append-prompt":
			m.mu.Lock()
			block := m.appendBlock || m.appendBlockOnce
			m.appendBlockOnce = false
			falseResponse, status, responseBody := m.appendFalse, m.appendStatus, m.appendBody
			m.mu.Unlock()
			if block {
				<-r.Context().Done()
				return
			}
			if falseResponse {
				writeOpenCodeMockResponse(w, http.StatusOK, "false")
			} else {
				writeOpenCodeMockResponse(w, status, responseBody)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/tui/submit-prompt":
			m.mu.Lock()
			block, falseResponse, status, responseBody, markerOnSubmit := m.submitBlock, m.submitFalse, m.submitStatus, m.submitBody, m.markerOnSubmit
			if markerOnSubmit {
				m.historyMarker = true
				m.historyExtra = append(m.historyExtra, m.historyExtraOnSubmit...)
				m.historyExtraOnSubmit = nil
			}
			onSubmit := m.onSubmit
			m.mu.Unlock()
			if onSubmit != nil {
				onSubmit()
			}
			if block {
				<-r.Context().Done()
				return
			}
			if falseResponse {
				writeOpenCodeMockResponse(w, http.StatusOK, "false")
			} else {
				writeOpenCodeMockResponse(w, status, responseBody)
			}
		default:
			writeOpenCodeMockResponse(w, http.StatusNotFound, "not found")
		}
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

func writeOpenCodeMockResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (m *openCodeMock) requestsSnapshot() []openCodeMockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]openCodeMockRequest(nil), m.requests...)
}

func (m *openCodeMock) appendCount() int {
	count := 0
	for _, request := range m.requestsSnapshot() {
		if request.Method == http.MethodPost && request.Path == "/tui/append-prompt" {
			count++
		}
	}
	return count
}

func nativeOpenCodeDeliveryFixture(t *testing.T, endpoint, thread, messageID string) (*Service, string, Session, Run, Message) {
	t.Helper()
	s, workspace := fixture(t)
	agent, worktree := worker(t, s, workspace, "planner")
	session, err := s.StartSession(context.Background(), workspace, SessionOptions{Agent: agent.ID, Worktree: worktree.ID})
	if err != nil {
		t.Fatal(err)
	}
	message := Message{ID: messageID, FromAgent: "orchestrator", ToAgent: agent.ID, Kind: "handoff", Body: "Please inspect the durable handoff."}
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		p, err := findSession(d, session.ID)
		if err != nil {
			return err
		}
		r, err := currentRun(d, p)
		if err != nil {
			return err
		}
		p.ClientSnapshot = Client{Adapter: "opencode", NativeDelivery: true}
		p.ClientThreadID = thread
		p.State = "running"
		r.State = "running"
		r.ClientThreadID = thread
		r.OpenCodeEndpoint = endpoint
		d.Registry.Messages = append(d.Registry.Messages, message)
		d.syncSession(p)
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var currentSession Session
	for _, candidate := range status.Sessions {
		if candidate.ID == session.ID {
			currentSession = candidate
			break
		}
	}
	var currentRun Run
	for _, candidate := range status.Runs {
		if candidate.ID == currentSession.CurrentRunID {
			currentRun = candidate
			break
		}
	}
	if currentSession.ID == "" || currentRun.ID == "" {
		t.Fatalf("native fixture did not produce an active Run: session=%+v run=%+v", currentSession, currentRun)
	}
	return s, workspace, currentSession, currentRun, message
}

func deliveryState(t *testing.T, s *Service, workspace, messageID, runID string) (DeliveryAttempt, Message) {
	t.Helper()
	var attempt DeliveryAttempt
	var message Message
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		for _, candidate := range d.Registry.Deliveries {
			if candidate.MessageID == messageID && candidate.RunID == runID {
				attempt = candidate
			}
		}
		found, err := findMessage(d, messageID)
		if err != nil {
			return err
		}
		message = *found
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return attempt, message
}

func TestOpenCodeNativeDeliveryUsesVisibleTUIContract(t *testing.T) {
	thread := "ses_visible"
	messageID := "msg_visible"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.markerOnSubmit = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)

	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err != nil {
		t.Fatal(err)
	}
	requests := mock.requestsSnapshot()
	if len(requests) != 6 {
		t.Fatalf("request order = %#v", requests)
	}
	want := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/global/health"},
		{http.MethodGet, "/session/" + thread + "/message?limit=100"},
		{http.MethodPost, "/tui/select-session"},
		{http.MethodPost, "/tui/append-prompt"},
		{http.MethodPost, "/tui/submit-prompt"},
		{http.MethodGet, "/session/" + thread + "/message?limit=100"},
	}
	for i, expected := range want {
		if requests[i].Method != expected.method || requests[i].Path != expected.path {
			t.Fatalf("request %d = %#v, want %s %s", i, requests[i], expected.method, expected.path)
		}
	}
	var selectBody map[string]string
	if err := json.Unmarshal([]byte(requests[2].Body), &selectBody); err != nil {
		t.Fatal(err)
	}
	if selectBody["sessionID"] != thread {
		t.Fatalf("selected session = %#v, want %q", selectBody, thread)
	}
	var appendBody map[string]string
	if err := json.Unmarshal([]byte(requests[3].Body), &appendBody); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(appendBody["text"], marker) || !strings.HasPrefix(appendBody["text"], "\n\n--- workspace handoff ---") || !strings.HasSuffix(appendBody["text"], "--- end workspace handoff ---\n\n") {
		t.Fatalf("append prompt did not preserve visible handoff separation: %q", appendBody["text"])
	}
	if requests[4].Body != "{}" {
		t.Fatalf("submit body = %q, want {}", requests[4].Body)
	}
	if strings.Contains(string(mustJSON(t, requests)), "prompt_async") || strings.Contains(string(mustJSON(t, requests)), "clear-prompt") {
		t.Fatal("native delivery used a forbidden route")
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseConfirmed || delivered.DeliveredRunID != run.ID || delivered.AcknowledgedAt != nil {
		t.Fatalf("delivery state = %+v message = %+v", attempt, delivered)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOpenCodeMarkerPrecheckSkipsTUIMutations(t *testing.T) {
	thread := "ses_already_delivered"
	messageID := "msg_already_delivered"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.historyMarker = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)

	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err != nil {
		t.Fatal(err)
	}
	for _, request := range mock.requestsSnapshot() {
		if request.Method != http.MethodGet {
			t.Fatalf("marker precheck made TUI mutation: %#v", request)
		}
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseConfirmed || delivered.DeliveredRunID != run.ID {
		t.Fatalf("delivery state = %+v message = %+v", attempt, delivered)
	}
}

func TestOpenCodeRetryAfterAppendResumesAtSubmit(t *testing.T) {
	thread := "ses_append_retry"
	messageID := "msg_append_retry"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.submitFalse = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)

	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err == nil {
		t.Fatal("rejected submit unexpectedly delivered")
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseAppended || delivered.DeliveredRunID != "" {
		t.Fatalf("after rejected submit state = %+v message = %+v", attempt, delivered)
	}
	mock.mu.Lock()
	mock.submitFalse = false
	mock.markerOnSubmit = true
	mock.mu.Unlock()
	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err != nil {
		t.Fatal(err)
	}
	if count := mock.appendCount(); count != 1 {
		t.Fatalf("append count = %d, want one append across retry", count)
	}
	_, delivered = deliveryState(t, s, workspace, messageID, run.ID)
	if delivered.DeliveredRunID != run.ID {
		t.Fatalf("retry did not deliver to current Run: %+v", delivered)
	}
}

func TestOpenCodeUncertainSubmitRetryNeverAppendsAgain(t *testing.T) {
	thread := "ses_submit_retry"
	messageID := "msg_submit_retry"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.submitBlock = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := s.deliverOpenCodeMessage(ctx, workspace, session, run, message)
	cancel()
	if err == nil {
		t.Fatal("blocked submit unexpectedly succeeded")
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseUncertainSubmit || delivered.DeliveredRunID != "" {
		t.Fatalf("after uncertain submit state = %+v message = %+v", attempt, delivered)
	}
	mock.mu.Lock()
	mock.submitBlock = false
	mock.markerOnSubmit = true
	mock.mu.Unlock()
	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err != nil {
		t.Fatal(err)
	}
	if count := mock.appendCount(); count != 1 {
		t.Fatalf("append count = %d, want one append across uncertain submit retry", count)
	}
}

func TestOpenCodeAppendTimeoutIsDurableAndDoesNotDuplicate(t *testing.T) {
	thread := "ses_append_uncertain"
	messageID := "msg_append_uncertain"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.appendBlock = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)

	started := time.Now()
	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err == nil {
		t.Fatal("blocked append unexpectedly succeeded")
	} else if elapsed := time.Since(started); elapsed > openCodeDeliveryTimeout+openCodePersistenceTimeout {
		t.Fatalf("append timeout took %s, exceeds bounded attempt", elapsed)
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseUncertainAppend || delivered.DeliveredRunID != "" {
		t.Fatalf("after uncertain append state = %+v message = %+v", attempt, delivered)
	}
	requestsBefore := len(mock.requestsSnapshot())
	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err == nil {
		t.Fatal("uncertain append was retried blindly")
	}
	requestsAfter := mock.requestsSnapshot()[requestsBefore:]
	for _, request := range requestsAfter {
		if strings.HasPrefix(request.Path, "/tui/") {
			t.Fatalf("uncertain append retry made TUI mutation: %#v", request)
		}
	}
}

func TestOpenCodeFalseMutationResponseLeavesMessageUndelivered(t *testing.T) {
	for _, test := range []struct {
		name        string
		configure   func(*openCodeMock)
		wantPhase   string
		wantAppends int
	}{
		{name: "append false", configure: func(m *openCodeMock) { m.appendFalse = true }, wantPhase: openCodeDeliveryPhaseRetry, wantAppends: 1},
		{name: "append non-2xx", configure: func(m *openCodeMock) { m.appendStatus, m.appendBody = http.StatusBadGateway, "rejected" }, wantPhase: openCodeDeliveryPhaseRetry, wantAppends: 1},
		{name: "submit rejected", configure: func(m *openCodeMock) { m.submitFalse = true }, wantPhase: openCodeDeliveryPhaseAppended, wantAppends: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread := "ses_false_" + strings.ReplaceAll(test.name, " ", "_")
			messageID := "msg_false_" + strings.ReplaceAll(test.name, " ", "_")
			mock := newOpenCodeMock(t, thread, "[workspace-message-id:"+messageID+"]")
			test.configure(mock)
			s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)
			if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err == nil {
				t.Fatal("rejected mutation unexpectedly succeeded")
			}
			attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
			if attempt.Phase != test.wantPhase || delivered.DeliveredRunID != "" || mock.appendCount() != test.wantAppends {
				t.Fatalf("state = %+v message = %+v append_count = %d", attempt, delivered, mock.appendCount())
			}
		})
	}
}

func TestOpenCodeLegacyCheckingPhaseIsRecoverable(t *testing.T) {
	thread := "ses_checking"
	messageID := "msg_checking"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.markerOnSubmit = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		d.Registry.Deliveries = append(d.Registry.Deliveries, DeliveryAttempt{MessageID: messageID, RunID: run.ID, Phase: openCodeDeliveryPhaseChecking})
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err != nil {
		t.Fatal(err)
	}
	attempt, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if attempt.Phase != openCodeDeliveryPhaseConfirmed || delivered.DeliveredRunID != run.ID {
		t.Fatalf("legacy checking state did not recover: %+v %+v", attempt, delivered)
	}
}

func TestOpenCodeStaleRunCannotRecordDelivery(t *testing.T) {
	thread := "ses_stale"
	messageID := "msg_stale"
	marker := "[workspace-message-id:" + messageID + "]"
	mock := newOpenCodeMock(t, thread, marker)
	mock.markerOnSubmit = true
	s, workspace, session, run, message := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)
	mock.mu.Lock()
	mock.onSubmit = func() {
		if err := s.With(context.Background(), workspace, func(d *Document) error {
			p, err := findSession(d, session.ID)
			if err != nil {
				return err
			}
			old, err := findRun(d, run.ID)
			if err != nil {
				return err
			}
			old.State = "interrupted"
			newRun := Run{ID: "run_successor", SessionID: p.ID, State: "running", ClientThreadID: thread, OpenCodeEndpoint: mock.server.URL, CreatedAt: nowUTC()}
			d.Registry.Runs = append(d.Registry.Runs, newRun)
			p.CurrentRunID = newRun.ID
			d.syncSession(p)
			return saveDocument(d)
		}); err != nil {
			t.Errorf("replace Run: %v", err)
		}
	}
	mock.mu.Unlock()

	if err := s.deliverOpenCodeMessage(context.Background(), workspace, session, run, message); err == nil {
		t.Fatal("stale delivery unexpectedly succeeded")
	}
	_, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if delivered.DeliveredRunID != "" || delivered.DeliveredSessionID != "" {
		t.Fatalf("stale Run recorded delivery: %+v", delivered)
	}
}

func TestOpenCodeTimeoutDoesNotStarveLaterMessage(t *testing.T) {
	thread := "ses_later_message"
	firstID := "msg_first_timeout"
	secondID := "msg_second"
	firstMarker := "[workspace-message-id:" + firstID + "]"
	secondMarker := "[workspace-message-id:" + secondID + "]"
	mock := newOpenCodeMock(t, thread, firstMarker)
	mock.appendBlockOnce = true
	mock.markerOnSubmit = true
	mock.historyExtraOnSubmit = []string{secondMarker}
	s, workspace, session, run, _ := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, firstID)
	second := Message{ID: secondID, FromAgent: "orchestrator", ToAgent: session.AgentID, Kind: "note", Body: "A later message must still be attempted."}
	if err := s.With(context.Background(), workspace, func(d *Document) error {
		d.Registry.Messages = append(d.Registry.Messages, second)
		return saveDocument(d)
	}); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("later message was starved for %s", elapsed)
	}
	if mock.appendCount() != 2 {
		t.Fatalf("append count = %d, want first timeout plus later message", mock.appendCount())
	}
	_, delivered := deliveryState(t, s, workspace, secondID, run.ID)
	if delivered.DeliveredRunID != run.ID {
		t.Fatalf("later message was not delivered after first timeout: %+v", delivered)
	}
}

func TestOpenCodeFailureUsesRunScopedFallbackNotificationOnce(t *testing.T) {
	thread := "ses_notification"
	messageID := "msg_notification"
	mock := newOpenCodeMock(t, thread, "[workspace-message-id:"+messageID+"]")
	mock.appendFalse = true
	s, workspace, session, run, _ := nativeOpenCodeDeliveryFixture(t, mock.server.URL, thread, messageID)
	base := s.Runtime.(*fakeRuntime)
	notifier := &fakeNotificationRuntime{fakeRuntime: base}
	s.Runtime = notifier

	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notifier.displayCount() != 1 {
		t.Fatalf("fallback notification count = %d, want one per Run", notifier.displayCount())
	}
	_, delivered := deliveryState(t, s, workspace, messageID, run.ID)
	if delivered.DeliveredRunID != "" || delivered.AcknowledgedAt != nil || delivered.NotifiedRunID != run.ID || delivered.NotifiedSessionID != session.ID {
		t.Fatalf("fallback changed the wrong message state: %+v", delivered)
	}
}
