package terminal

import (
	"context"
	"errors"
	"testing"

	"workspace/internal/core"
)

type fakeProcessStarter struct {
	name string
	args []string
	err  error
}

func (s *fakeProcessStarter) Start(name string, args ...string) error {
	s.name = name
	s.args = append([]string(nil), args...)
	return s.err
}

func TestWSLLauncherUsesSeparateWindowsTerminalArgv(t *testing.T) {
	starter := &fakeProcessStarter{}
	launcher := WSLLauncher{
		Env: func(key string) string {
			if key == "WSL_DISTRO_NAME" {
				return "Ubuntu Test"
			}
			return ""
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		Starter:  starter,
	}

	err := launcher.Launch(context.Background(), LaunchSpec{Socket: "socket with spaces", Session: "workspace-viewer-abc"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-w", "_new", "wsl.exe", "-d", "Ubuntu Test", "--exec", "tmux", "-L", "socket with spaces", "attach-session", "-t", "=workspace-viewer-abc"}
	if starter.name != "wt.exe" {
		t.Fatalf("launcher executable = %q, want wt.exe", starter.name)
	}
	if len(starter.args) != len(want) {
		t.Fatalf("launcher argv = %#v, want %#v", starter.args, want)
	}
	for i := range want {
		if starter.args[i] != want[i] {
			t.Fatalf("launcher argv = %#v, want %#v", starter.args, want)
		}
	}
}

func TestWSLLauncherReportsUnavailableAndStartFailure(t *testing.T) {
	noWindowsTerminal := WSLLauncher{
		Env:      func(string) string { return "Ubuntu" },
		LookPath: func(name string) (string, error) { return "", errors.New(name + " missing") },
		Starter:  &fakeProcessStarter{},
	}
	if err := noWindowsTerminal.Launch(context.Background(), LaunchSpec{Session: "viewer"}); !hasCoreCode(err, "terminal_launcher_unavailable") {
		t.Fatalf("missing launcher error = %v", err)
	}

	startFailure := &fakeProcessStarter{err: errors.New("permission denied")}
	launcher := WSLLauncher{
		Env: func(key string) string {
			if key == "WSL_DISTRO_NAME" {
				return "Ubuntu"
			}
			return ""
		},
		LookPath: func(string) (string, error) { return "found", nil },
		Starter:  startFailure,
	}
	if err := launcher.Launch(context.Background(), LaunchSpec{Session: "viewer"}); !hasCoreCode(err, "terminal_launch_failed") {
		t.Fatalf("start failure = %v", err)
	}
}

func TestWSLLauncherRequiresDistribution(t *testing.T) {
	launcher := WSLLauncher{
		Env:      func(string) string { return "" },
		LookPath: func(string) (string, error) { return "found", nil },
		Starter:  &fakeProcessStarter{},
	}
	if err := launcher.Launch(context.Background(), LaunchSpec{Session: "viewer"}); !hasCoreCode(err, "terminal_launcher_unavailable") {
		t.Fatalf("missing distribution error = %v", err)
	}
}

func hasCoreCode(err error, code string) bool {
	var coreErr *core.Error
	return errors.As(err, &coreErr) && coreErr.Code == code
}
