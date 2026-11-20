//go:build unix

package ci

import (
	"os/exec"
	"syscall"
)

// ownGroup puts the tool in its own process group and kills the whole group on cancel: a tool that
// starts helpers (golangci-lint runs go list) must not leave them holding its output open.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
