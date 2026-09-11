package core

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeQuestionAnswerPreservesProtocolIdentity(t *testing.T) {
	r := rpcMessage{Method: "item/tool/requestUserInput", Params: json.RawMessage(`{"questions":[{"id":"environment","question":"Which environment?","options":[{"label":"Local","description":"Use the local app"}]}]}`)}
	var out bytes.Buffer
	showNativeRequest(&out, "42", r)
	if !strings.Contains(out.String(), "Which environment?") || !strings.Contains(out.String(), "/answer 42") {
		t.Fatal("question not rendered")
	}
	answer, err := nativeAnswer(r, "Local")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(answer)
	if string(b) != `{"answers":{"environment":{"answers":["Local"]}}}` {
		t.Fatalf("invalid answer: %s", b)
	}
	r.Method = "item/commandExecution/requestApproval"
	_, err = nativeAnswer(r, "Local")
	expectCode(t, err, "invalid_answer")
}
