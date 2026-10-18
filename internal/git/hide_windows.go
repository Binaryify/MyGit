package git

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps git from flashing a console window for each call.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
