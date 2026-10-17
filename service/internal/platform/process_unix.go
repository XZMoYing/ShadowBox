//go:build !windows

package platform

import "os/exec"

func PrepareCommand(cmd *exec.Cmd) {}
