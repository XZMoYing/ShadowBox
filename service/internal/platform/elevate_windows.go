package platform

import (
	"os/exec"
	"strings"
)

func Elevate(binary string, args ...string) *exec.Cmd {
	esc := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	parts := []string{}
	for _, s := range args {
		parts = append(parts, esc(`"`+s+`"`))
	}
	script := `$p=Start-Process -FilePath ` + esc(binary) + ` -ArgumentList @(` + strings.Join(parts, ",") + `) -Verb RunAs -WindowStyle Hidden -Wait -PassThru;exit $p.ExitCode`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	PrepareCommand(cmd)
	return cmd
}
