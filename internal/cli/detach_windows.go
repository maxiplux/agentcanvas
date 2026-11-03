//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// detach makes cmd outlive the process that starts it: no console and its own
// process group, so the agent closing attach does not stop the studio.
func detach(cmd *exec.Cmd) {
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup, HideWindow: true}
}
