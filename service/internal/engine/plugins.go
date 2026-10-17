package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"shadowbox/service/internal/platform"
	"shadowbox/service/internal/profile"
)

type PluginProcess struct {
	cmd    *exec.Cmd
	config string
	done   chan struct{}
	err    error
}

func StartPlugins(chain []profile.Node, plugins []profile.Plugin, dir string) (map[string]int, []*PluginProcess, error) {
	ports := map[string]int{}
	processes := []*PluginProcess{}
	fail := func(e error) (map[string]int, []*PluginProcess, error) { StopPlugins(processes); return nil, nil, e }
	for _, n := range chain {
		if n.External == nil {
			continue
		}
		if e := ValidateExternal(n.External); e != nil {
			return fail(e)
		}
		var selected *profile.Plugin
		for i := range plugins {
			if plugins[i].Kind == n.External.Kind {
				selected = &plugins[i]
				break
			}
		}
		if selected == nil {
			return fail(fmt.Errorf("请先在插件设置中安装 %s", n.External.Kind))
		}
		st, e := os.Stat(selected.Path)
		if e != nil || st.IsDir() {
			return fail(fmt.Errorf("插件文件不可用：%s", selected.Name))
		}
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return fail(e)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		cfg := profile.CloneMap(n.External.Config)
		path := filepath.Join(dir, "plugin-"+profile.NewID()+".json")
		args := []string{}
		env := os.Environ()
		switch n.External.Kind {
		case "naive":
			proxy, _ := cfg["proxy"].(string)
			cfg["proxy"] = strings.TrimPrefix(proxy, "naive+")
			cfg["listen"] = fmt.Sprintf("socks://127.0.0.1:%d", port)
			args = []string{path}
		case "trojan-go":
			cfg["run_type"] = "client"
			cfg["local_addr"] = "127.0.0.1"
			cfg["local_port"] = port
			args = []string{"-config", path}
		case "mieru":
			cfg["socks5Port"] = port
			cfg["socks5ListenLAN"] = false
			args = []string{"run"}
			env = append(env, "MIERU_CONFIG_JSON_FILE="+path)
		default:
			return fail(fmt.Errorf("不支持的插件类型 %s", n.External.Kind))
		}
		data, e := json.Marshal(cfg)
		if e != nil {
			return fail(e)
		}
		if e = os.WriteFile(path, data, 0600); e != nil {
			return fail(e)
		}
		cmd := exec.Command(selected.Path, args...)
		platform.PrepareCommand(cmd)
		cmd.Env = env
		cmd.Stdout = nil
		cmd.Stderr = nil
		if e = cmd.Start(); e != nil {
			_ = os.Remove(path)
			return fail(e)
		}
		p := &PluginProcess{cmd: cmd, config: path, done: make(chan struct{})}
		go func() { p.err = cmd.Wait(); close(p.done) }()
		processes = append(processes, p)
		deadline := time.Now().Add(8 * time.Second)
		ready := false
		for time.Now().Before(deadline) {
			select {
			case <-p.done:
				return fail(fmt.Errorf("%s 插件提前退出：%v", selected.Name, p.err))
			default:
			}
			c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
			if e == nil {
				c.Close()
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			return fail(fmt.Errorf("%s 插件启动超时", selected.Name))
		}
		ports[n.ID] = port
	}
	return ports, processes, nil
}
func StopPlugins(processes []*PluginProcess) {
	for _, p := range processes {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			select {
			case <-p.done:
			case <-time.After(time.Second):
			}
		}
		_ = os.Remove(p.config)
	}
}

func (p *PluginProcess) Done() <-chan struct{} { return p.done }
