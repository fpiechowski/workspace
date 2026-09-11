//go:build !windows

package core

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
