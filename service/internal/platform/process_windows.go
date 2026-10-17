package platform

import (
	"os/exec"
	"syscall"
)

func PrepareCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
