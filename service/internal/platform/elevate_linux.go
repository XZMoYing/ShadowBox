package platform

import "os/exec"

func Elevate(binary string, args ...string) *exec.Cmd {
	return exec.Command("pkexec", append([]string{binary}, args...)...)
}
