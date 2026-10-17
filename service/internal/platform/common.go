package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ProxySnapshot struct {
	Entries []ProxyEntry `json:"entries"`
}
type ProxyEntry struct {
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Raw     string `json:"raw,omitempty"`
}

func DataDir() string {
	if p := os.Getenv("SHADOWBOX_DATA_DIR"); p != "" {
		return p
	}
	p, e := os.UserConfigDir()
	if e != nil {
		p = os.TempDir()
	}
	return filepath.Join(p, "ShadowBox")
}
func SaveSnapshot(dir string, s ProxySnapshot) error {
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, "proxy-recovery.json"), b, 0600)
}
func LoadSnapshot(dir string) (ProxySnapshot, error) {
	b, e := os.ReadFile(filepath.Join(dir, "proxy-recovery.json"))
	var s ProxySnapshot
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func ClearSnapshot(dir string) error { return os.Remove(filepath.Join(dir, "proxy-recovery.json")) }
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	PrepareCommand(cmd)
	b, e := cmd.CombinedOutput()
	if e != nil {
		return string(b), fmt.Errorf("%s: %s (%w)", name, strings.TrimSpace(string(b)), e)
	}
	return strings.TrimSpace(string(b)), nil
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
