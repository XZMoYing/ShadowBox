package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func Snapshot() (ProxySnapshot, error) {
	s := ProxySnapshot{}
	v, e := run("/usr/sbin/networksetup", "-listallnetworkservices")
	if e != nil {
		return s, e
	}
	for _, line := range strings.Split(v, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		for _, kind := range []string{"webproxy", "securewebproxy", "socksfirewallproxy"} {
			raw, e := run("/usr/sbin/networksetup", "-get"+kind, line)
			if e != nil {
				return s, e
			}
			en := ProxyEntry{Service: line, Kind: kind}
			for _, r := range strings.Split(raw, "\n") {
				k, val, ok := strings.Cut(r, ": ")
				if !ok {
					continue
				}
				switch k {
				case "Enabled":
					en.Enabled = val == "Yes"
				case "Server":
					en.Host = val
				case "Port":
					en.Port, _ = strconv.Atoi(val)
				case "Authenticated Proxy Enabled":
					en.Raw = val
				}
			}
			s.Entries = append(s.Entries, en)
		}
		raw, e := run("/usr/sbin/networksetup", "-getautoproxyurl", line)
		if e != nil {
			return s, e
		}
		pac := ProxyEntry{Service: line, Kind: "pac"}
		for _, v := range strings.Split(raw, "\n") {
			k, value, ok := strings.Cut(v, ": ")
			if !ok {
				continue
			}
			if k == "Enabled" {
				pac.Enabled = value == "Yes"
			}
			if k == "URL" {
				pac.Host = value
			}
		}
		s.Entries = append(s.Entries, pac)
	}
	return s, nil
}

func networkChanges(commands [][]string) error {
	if len(commands) == 0 {
		return nil
	}
	for _, args := range commands {
		out, e := run("/usr/sbin/networksetup", args...)
		if e == nil && (strings.Contains(out, "Error:") || strings.Contains(out, "administrator access") || strings.Contains(out, "root privileges")) {
			e = fmt.Errorf("%s", out)
		}
		if e == nil {
			continue
		}
		lower := strings.ToLower(e.Error())
		if !strings.Contains(lower, "admin") && !strings.Contains(lower, "root") && !strings.Contains(lower, "permission") && !strings.Contains(lower, "authoriz") {
			return e
		}
		// Some users or security settings require administrator authorization. Only
		// these fixed networksetup commands run elevated; the service remains normal.
		scriptCommands := []string{}
		for _, args := range commands {
			words := []string{"/usr/sbin/networksetup"}
			for _, arg := range args {
				words = append(words, shellQuote(arg))
			}
			scriptCommands = append(scriptCommands, strings.Join(words, " "))
		}
		script := "do shell script " + strconv.Quote(strings.Join(scriptCommands, " && ")) + " with administrator privileges"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
		if err != nil {
			return fmt.Errorf("系统代理授权或更改失败：%s (%w)", strings.TrimSpace(string(result)), err)
		}
		if strings.Contains(string(result), "Error:") {
			return fmt.Errorf("系统代理更改失败：%s", strings.TrimSpace(string(result)))
		}
		return nil
	}
	return nil
}
func Apply(port int, s ProxySnapshot) error {
	if len(s.Entries) == 0 {
		return fmt.Errorf("未找到可接管的 macOS 网络服务")
	}
	commands := [][]string{}
	for _, x := range s.Entries {
		if x.Kind == "pac" {
			if x.Enabled {
				return fmt.Errorf("网络服务 %s 使用 PAC 自动代理，请先关闭 PAC 或选择 TUN/仅本地代理", x.Service)
			}
			continue
		}
		if x.Raw == "1" {
			return fmt.Errorf("网络服务 %s 保留了认证代理，请先手动处理该配置以避免丢失凭据", x.Service)
		}
	}
	for _, x := range s.Entries {
		if x.Kind == "pac" {
			continue
		}
		commands = append(commands, []string{"-set" + x.Kind, x.Service, "127.0.0.1", strconv.Itoa(port)}, []string{"-set" + x.Kind + "state", x.Service, "on"})
	}
	return networkChanges(commands)
}
func Restore(s ProxySnapshot, port int) error {
	current, e := Snapshot()
	if e != nil {
		return e
	}
	commands := [][]string{}
	for _, old := range s.Entries {
		if old.Kind == "pac" {
			continue
		}
		owned := false
		for _, now := range current.Entries {
			if now.Service == old.Service && now.Kind == old.Kind && now.Host == "127.0.0.1" && now.Port == port && now.Raw != "1" {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if old.Host != "" && old.Port > 0 {
			commands = append(commands, []string{"-set" + old.Kind, old.Service, old.Host, strconv.Itoa(old.Port)})
		}
		state := "off"
		if old.Enabled {
			state = "on"
		}
		commands = append(commands, []string{"-set" + old.Kind + "state", old.Service, state})
	}
	return networkChanges(commands)
}
