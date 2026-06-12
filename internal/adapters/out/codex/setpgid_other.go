//go:build !linux

package codex

import "os/exec"

// setParentDeathSignal is a no-op on non-Linux platforms that do not
// support Pdeathsig in SysProcAttr.
func setParentDeathSignal(cmd *exec.Cmd) {
	// No-op: platform does not support parent-death signal.
}
