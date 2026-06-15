//go:build linux

package codex

import (
	"os/exec"
	"syscall"
)

// setParentDeathSignal configures the child process to receive SIGTERM when
// the parent bundled runtime dies. This prevents orphaned app-server
// subprocesses when the runtime is killed abruptly.
//
// Linux provides Pdeathsig in SysProcAttr for this purpose. On other
// platforms this is a no-op (see setpgid_other.go).
func setParentDeathSignal(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
}
