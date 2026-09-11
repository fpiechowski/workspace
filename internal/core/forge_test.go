package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Invoked by temporary gh/glab/git wrappers, never by normal test discovery.
func TestForgeCommandProcess(t *testing.T) {
	if os.Getenv("WORKSPACE_FORGE_HELPER") != "1" {
		return
	}
	index := 0
	for i, arg := range os.Args {
		if arg == "--" {
			index = i + 1
			break
		}
	}
	args := os.Args[index:]
	if len(args) < 1 {
		os.Exit(2)
	}
	if args[0] == "command" {
		b, err := os.ReadFile(args[1])
		if err != nil {
			os.Exit(3)
		}
		var request struct {
			Version       int           `json:"version"`
			Action        string        `json:"action"`
			ChangeRequest ChangeRequest `json:"change_request"`
			BodyFile      string        `json:"body_file"`
		}
		if json.Unmarshal(b, &request) != nil || request.Version != 1 || request.ChangeRequest.ID != "cr_test" {
			os.Exit(4)
		}
		if request.Action == "lookup" {
			fmt.Print(`{"found":false}`)
		} else {
			body, err := os.ReadFile(request.BodyFile)
			if err != nil || string(body) != "line one\nline two\n" {
				os.Exit(5)
			}
			fmt.Print(`{"found":true,"result":{"id":"42","url":"https://forge.test/42","state":"open","head_commit":"abc"}}`)
		}
		os.Exit(0)
	}
	f, err := os.OpenFile(os.Getenv("WORKSPACE_FORGE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(6)
	}
	_ = json.NewEncoder(f).Encode(args)
	_ = f.Close()
	if len(args) > 2 && args[2] == "list" {
		body := "<!-- workspace-cr:cr_test -->"
		if os.Getenv("WORKSPACE_FORGE_CONFLICT") == "1" {
			body = "unrelated request"
		}
		if args[0] == "gh" {
			_ = json.NewEncoder(os.Stdout).Encode([]any{map[string]any{"number": 42, "url": "https://forge.test/42", "state": "OPEN", "headRefOid": "abc", "body": body}})
		} else {
			_ = json.NewEncoder(os.Stdout).Encode([]any{map[string]any{"iid": 42, "web_url": "https://forge.test/42", "state": "opened", "sha": "abc", "description": body}})
		}
	}
	os.Exit(0)
}

func TestConfiguredForgeCommandProtocol(t *testing.T) {
	t.Setenv("WORKSPACE_FORGE_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := CLIForge{Config: ForgeConfig{Adapter: "command", CommandArgv: []string{exe, "-test.run=TestForgeCommandProcess", "--", "command", "{request_file}"}}}
	ctx := context.Background()
	root := t.TempDir()
	cr := ChangeRequest{ID: "cr_test", HeadCommit: "abc"}
	if result, err := f.Lookup(ctx, root, cr); err != nil || result != nil {
		t.Fatal("lookup protocol", result, err)
	}
	body := filepath.Join(root, "description with spaces.md")
	if err := os.WriteFile(body, []byte("line one\nline two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := f.Publish(ctx, root, cr, body)
	if err != nil || result.ID != "42" || result.HeadCommit != "abc" {
		t.Fatal("publish protocol", result, err)
	}
}

func TestForgeCLIArgumentsAndLookupIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix runtime command wrappers")
	}
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "gh", "glab"} {
		script := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", "'\\''") + "' -test.run=TestForgeCommandProcess -- " + name + " \"$@\"\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKSPACE_FORGE_HELPER", "1")
	cr := ChangeRequest{ID: "cr_test", Branch: "workspace/test/task", Target: "main", HeadCommit: "abc", Title: "Title with ' quotes and $literal"}
	body := filepath.Join(root, "description with spaces.md")
	if err := os.WriteFile(body, []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []string{"github", "gitlab"} {
		t.Run(adapter, func(t *testing.T) {
			log := filepath.Join(root, adapter+".jsonl")
			t.Setenv("WORKSPACE_FORGE_LOG", log)
			f := CLIForge{Config: ForgeConfig{Adapter: adapter}}
			result, err := f.Publish(context.Background(), root, cr, body)
			if err != nil || result.ID != "42" || result.State != "open" {
				t.Fatal(result, err)
			}
			b, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			var calls [][]string
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				var args []string
				if err := json.Unmarshal([]byte(line), &args); err != nil {
					t.Fatal(err)
				}
				calls = append(calls, args)
			}
			if len(calls) != 3 || !reflect.DeepEqual(calls[0], []string{"git", "push", "origin", "abc:refs/heads/" + cr.Branch}) {
				t.Fatal("unexpected push", calls)
			}
			var expected []string
			if adapter == "github" {
				expected = []string{"gh", "pr", "create", "--title", cr.Title, "--body-file", body, "--base", "main", "--head", cr.Branch}
			} else {
				expected = []string{"glab", "mr", "create", "--title", cr.Title, "--description-file", body, "--target-branch", "main", "--source-branch", cr.Branch, "--yes"}
			}
			if !reflect.DeepEqual(calls[1], expected) {
				t.Fatal("unsafe or incorrect create arguments", calls[1])
			}
			t.Setenv("WORKSPACE_FORGE_CONFLICT", "1")
			_, err = f.Lookup(context.Background(), root, cr)
			expectCode(t, err, "forge_conflict")
		})
	}
}
