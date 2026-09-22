package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"workspace/internal/core"
)

// LaunchSpec identifies the already verified tmux viewer that a launcher must
// open. The values are passed as separate argv entries; they are never
// interpolated into a shell command.
type LaunchSpec struct {
	Socket  string
	Session string
}

// Launcher starts a separate terminal process and returns after that process
// has been started. It must not attach through the caller's stdin/stdout.
type Launcher interface {
	Launch(context.Context, LaunchSpec) error
}

// ProcessStarter is the small process boundary used by terminal launchers.
// Keeping it separate makes launcher argv deterministic in unit tests.
type ProcessStarter interface {
	Start(string, ...string) error
}

type OSProcessStarter struct{}

func (OSProcessStarter) Start(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// WSLLauncher opens Windows Terminal in a new Windows window and starts the
// current WSL distribution there. It is deliberately explicit about both the
// distribution and the tmux socket so a default or foreign server cannot be
// selected accidentally.
type WSLLauncher struct {
	Env      func(string) string
	LookPath func(string) (string, error)
	Starter  ProcessStarter
}

func (l WSLLauncher) env(key string) string {
	if l.Env != nil {
		return l.Env(key)
	}
	return os.Getenv(key)
}

func (l WSLLauncher) lookup(name string) (string, error) {
	if l.LookPath != nil {
		return l.LookPath(name)
	}
	return exec.LookPath(name)
}

func (l WSLLauncher) Launch(ctx context.Context, spec LaunchSpec) error {
	if err := validateLaunchSpec(spec); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	env := l.env
	distribution := strings.TrimSpace(env("WSL_DISTRO_NAME"))
	if distribution == "" {
		return launcherUnavailable("WSL_DISTRO_NAME is not available; run workspace from a WSL distribution")
	}
	lookup := l.lookup
	if _, err := lookup("wt.exe"); err != nil {
		return launcherUnavailable("Windows Terminal (wt.exe) was not found in the WSL PATH")
	}
	if _, err := lookup("wsl.exe"); err != nil {
		return launcherUnavailable("the WSL launcher (wsl.exe) was not found in the WSL PATH")
	}
	starter := l.Starter
	if starter == nil {
		starter = OSProcessStarter{}
	}
	args := append([]string{"-w", "_new", "wsl.exe", "-d", distribution, "--exec", "tmux"}, tmuxAttachArgs(spec)...)
	if err := starter.Start("wt.exe", args...); err != nil {
		return launcherFailed("Windows Terminal could not be started", err)
	}
	return nil
}

// UnixLauncher covers common Linux terminal launchers. WSL is handled by
// WSLLauncher so that it always uses a genuinely separate Windows Terminal
// window rather than a Linux terminal attached to the WSL console.
type UnixLauncher struct {
	GOOS     string
	LookPath func(string) (string, error)
	Starter  ProcessStarter
}

func (l UnixLauncher) Launch(ctx context.Context, spec LaunchSpec) error {
	if err := validateLaunchSpec(spec); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lookup := l.LookPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	starter := l.Starter
	if starter == nil {
		starter = OSProcessStarter{}
	}
	if l.GOOS == "darwin" {
		path, err := lookup("open")
		if err != nil {
			return launcherUnavailable("macOS Terminal launcher (open) was not found")
		}
		args := append([]string{"-a", "Terminal", "--args", "tmux"}, tmuxAttachArgs(spec)...)
		if err := starter.Start(path, args...); err != nil {
			return launcherFailed("macOS Terminal could not be started", err)
		}
		return nil
	}

	candidates := []struct {
		name   string
		prefix []string
	}{
		{name: "x-terminal-emulator", prefix: []string{"-e"}},
		{name: "gnome-terminal", prefix: []string{"--"}},
		{name: "konsole", prefix: []string{"-e"}},
		{name: "alacritty", prefix: []string{"-e"}},
	}
	for _, candidate := range candidates {
		path, err := lookup(candidate.name)
		if err != nil {
			continue
		}
		args := append(append([]string{}, candidate.prefix...), append([]string{"tmux"}, tmuxAttachArgs(spec)...)...)
		if err := starter.Start(path, args...); err != nil {
			return launcherFailed(candidate.name+" could not be started", err)
		}
		return nil
	}
	return launcherUnavailable("no supported terminal launcher was found (tried x-terminal-emulator, gnome-terminal, konsole, and alacritty)")
}

type autoLauncher struct {
	goos    string
	env     func(string) string
	lookup  func(string) (string, error)
	starter ProcessStarter
	procVer func() ([]byte, error)
}

// NewDefaultLauncher selects the platform launcher at operation time. No
// external process is started during construction, which keeps TUI startup and
// launcher discovery failures separate.
func NewDefaultLauncher() Launcher {
	return autoLauncher{
		goos:    runtime.GOOS,
		env:     os.Getenv,
		lookup:  exec.LookPath,
		starter: OSProcessStarter{},
		procVer: func() ([]byte, error) { return os.ReadFile("/proc/version") },
	}
}

func (l autoLauncher) Launch(ctx context.Context, spec LaunchSpec) error {
	if l.goos == "linux" && isWSL(l.env, l.procVer) {
		return WSLLauncher{Env: l.env, LookPath: l.lookup, Starter: l.starter}.Launch(ctx, spec)
	}
	if l.goos == "linux" || l.goos == "darwin" {
		return UnixLauncher{GOOS: l.goos, LookPath: l.lookup, Starter: l.starter}.Launch(ctx, spec)
	}
	return launcherUnavailable("dedicated terminal opening is supported on WSL with Windows Terminal, Linux desktop terminals, and macOS Terminal")
}

func isWSL(env func(string) string, procVer func() ([]byte, error)) bool {
	if env != nil && (strings.TrimSpace(env("WSL_DISTRO_NAME")) != "" || strings.TrimSpace(env("WSL_INTEROP")) != "") {
		return true
	}
	if procVer == nil {
		return false
	}
	version, err := procVer()
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(version))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func validateLaunchSpec(spec LaunchSpec) error {
	if strings.TrimSpace(spec.Session) == "" {
		return &core.Error{Code: "invalid_navigation_target", Message: "dedicated terminal session is required"}
	}
	return nil
}

func tmuxAttachArgs(spec LaunchSpec) []string {
	socket := spec.Socket
	if strings.TrimSpace(socket) == "" {
		socket = "default"
	}
	return []string{"-L", socket, "attach-session", "-t", "=" + spec.Session}
}

func launcherUnavailable(message string) error {
	return &core.Error{Code: "terminal_launcher_unavailable", Message: message}
}

func launcherFailed(message string, err error) error {
	return &core.Error{Code: "terminal_launch_failed", Message: fmt.Sprintf("%s: %v", message, err)}
}
