package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"shadowbox/service/internal/platform"
	"shadowbox/service/internal/profile"
)

type TunProcess struct {
	cmd       *exec.Cmd
	prefix    string
	done      chan struct{}
	err       error
	once      sync.Once
	heartbeat chan struct{}
}

func StartTun(data []byte, dir string) (*TunProcess, error) {
	if e := ValidatePrivileged(data); e != nil {
		return nil, e
	}
	binary, e := os.Executable()
	if e != nil {
		return nil, e
	}
	prefix := filepath.Join(dir, "tun-"+profile.NewID())
	if e = os.WriteFile(prefix+".json", data, 0600); e != nil {
		return nil, e
	}
	p := &TunProcess{prefix: prefix, done: make(chan struct{}), heartbeat: make(chan struct{})}
	_ = os.WriteFile(prefix+".beat", []byte("alive"), 0600)
	p.cmd = platform.Elevate(binary, "tun-worker", "--session", prefix)
	if e = p.cmd.Start(); e != nil {
		p.cleanup()
		return nil, e
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-p.heartbeat:
				return
			case <-tick.C:
				_ = os.Chtimes(prefix+".beat", time.Now(), time.Now())
			}
		}
	}()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if b, e := os.ReadFile(prefix + ".ready"); e == nil {
			var state struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(b, &state) == nil {
				if state.Error != "" {
					p.Close()
					return nil, fmt.Errorf("TUN 启动失败：%s", state.Error)
				}
				return p, nil
			}
		}
		select {
		case <-p.done:
			p.Close()
			return nil, fmt.Errorf("TUN 授权或启动失败：%v", p.err)
		default:
		}
		time.Sleep(150 * time.Millisecond)
	}
	p.Close()
	return nil, fmt.Errorf("TUN 授权等待超时")
}
func (p *TunProcess) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() { close(p.heartbeat) })
	_ = os.WriteFile(p.prefix+".stop", []byte("stop"), 0600)
	select {
	case <-p.done:
	case <-time.After(8 * time.Second):
	}
	p.cleanup()
}
func (p *TunProcess) Done() <-chan struct{} { return p.done }

func (p *TunProcess) cleanup() {
	for _, s := range []string{".json", ".ready", ".stop", ".beat", ".stats"} {
		_ = os.Remove(p.prefix + s)
	}
}
func (p *TunProcess) Stats() map[string]any {
	m := map[string]any{}
	if b, e := os.ReadFile(p.prefix + ".stats"); e == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}
func RunTunWorker(prefix string) error {
	// Only app-generated, private session files are accepted. No plugin is ever started elevated.
	st, e := os.Lstat(prefix + ".json")
	if e != nil {
		return e
	}
	if !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0) || st.Size() > 16<<20 {
		return fmt.Errorf("TUN 会话配置权限或大小不正确")
	}
	data, e := os.ReadFile(prefix + ".json")
	if e != nil {
		return e
	}
	if e = ValidatePrivileged(data); e != nil {
		return e
	}
	i, e := Start(data, nil)
	ready := map[string]string{}
	if e != nil {
		ready["error"] = e.Error()
	}
	b, _ := json.Marshal(ready)
	_ = os.WriteFile(prefix+".ready", b, 0644)
	if e != nil {
		return e
	}
	defer i.Close()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for range tick.C {
		if _, e = os.Stat(prefix + ".stop"); e == nil {
			return nil
		}
		beat, e := os.Stat(prefix + ".beat")
		if e != nil || time.Since(beat.ModTime()) > 5*time.Second {
			return nil
		}
		b, _ = json.Marshal(i.Stats(false))
		_ = os.WriteFile(prefix+".stats", b, 0644)
	}
	return nil
}

// Recover stale sessions before starting another core. An orphaned worker also
// exits itself when its heartbeat is absent; the stop file makes recovery prompt.
func RecoverTun(dir string) {
	files, _ := filepath.Glob(filepath.Join(dir, "tun-*.json"))
	for _, file := range files {
		_ = os.WriteFile(strings.TrimSuffix(file, ".json")+".stop", []byte("stop"), 0600)
	}
	if len(files) > 0 {
		time.Sleep(1200 * time.Millisecond)
	}
	for _, file := range files {
		p := &TunProcess{prefix: strings.TrimSuffix(file, ".json")}
		p.cleanup()
	}
}
