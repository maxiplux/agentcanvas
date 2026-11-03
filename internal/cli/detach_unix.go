//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detach makes cmd outlive the process that starts it: a new session, so the
// agent closing attach (and its terminal) does not stop the studio.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
