package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestStructuredErrorsAndWorkflowDiscovery(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute([]string{"--json", "workflow", "list"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("%s", errOut.String())
	}
	var response struct {
		OK   bool `json:"ok"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || len(response.Data) != 1 || response.Data[0].ID != "plan-first" {
		t.Fatal("invalid workflow list")
	}
	out.Reset()
	errOut.Reset()
	code = Execute([]string{"--json", "--project", t.TempDir(), "status"}, nil, &out, &errOut)
	if code != 1 || !bytes.Contains(out.Bytes(), []byte(`"code":"project_not_found"`)) {
		t.Fatalf("expected structured error: %d %s", code, out.String())
	}
}
