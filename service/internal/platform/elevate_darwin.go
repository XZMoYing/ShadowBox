package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

func Elevate(binary string, args ...string) *exec.Cmd {
	parts := []string{shellQuote(binary)}
	for _, s := range args {
		parts = append(parts, shellQuote(s))
	}
	script := "do shell script " + strconv.Quote("exec "+strings.Join(parts, " ")) + " with administrator privileges"
	return exec.Command("/usr/bin/osascript", "-e", script)
}
