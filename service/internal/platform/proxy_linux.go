package platform

import (
	"fmt"
	"strconv"
	"strings"
)

func Snapshot() (ProxySnapshot, error) {
	s := ProxySnapshot{}
	mode, e := run("gsettings", "get", "org.gnome.system.proxy", "mode")
	if e != nil {
		return s, fmt.Errorf("此桌面暂不支持自动系统代理，请选择本地代理或 TUN：%w", e)
	}
	s.Entries = append(s.Entries, ProxyEntry{Kind: "mode", Raw: mode})
	auth, e := run("gsettings", "get", "org.gnome.system.proxy.http", "use-authentication")
	if e != nil {
		return s, e
	}
	s.Entries = append(s.Entries, ProxyEntry{Kind: "auth", Enabled: auth == "true"})
	for _, kind := range []string{"http", "https", "socks"} {
		schema := "org.gnome.system.proxy." + kind
		h, e := run("gsettings", "get", schema, "host")
		if e != nil {
			return s, e
		}
		p, e := run("gsettings", "get", schema, "port")
		if e != nil {
			return s, e
		}
		port, _ := strconv.Atoi(p)
		s.Entries = append(s.Entries, ProxyEntry{Kind: kind, Host: strings.Trim(h, "'"), Port: port})
	}
	return s, nil
}
func Apply(port int, s ProxySnapshot) error {
	for _, x := range s.Entries {
		if x.Kind == "auth" && x.Enabled {
			return fmt.Errorf("GNOME 配置了认证代理，请先手动处理或选择 TUN/仅本地代理")
		}
	}

	for _, kind := range []string{"http", "https", "socks"} {
		schema := "org.gnome.system.proxy." + kind
		if _, e := run("gsettings", "set", schema, "host", "127.0.0.1"); e != nil {
			return e
		}
		if _, e := run("gsettings", "set", schema, "port", strconv.Itoa(port)); e != nil {
			return e
		}
	}
	_, e := run("gsettings", "set", "org.gnome.system.proxy", "mode", "manual")
	return e
}
func Restore(s ProxySnapshot, port int) error {
	current, e := Snapshot()
	if e != nil {
		return e
	}
	by := map[string]ProxyEntry{}
	mode := ""
	for _, x := range current.Entries {
		by[x.Kind] = x
		if x.Kind == "mode" {
			mode = strings.Trim(x.Raw, "'")
		}
	}
	allUnchangedOrOwned := true
	for _, old := range s.Entries {
		if old.Kind == "mode" || old.Kind == "auth" {
			continue
		}
		now, ok := by[old.Kind]
		if !ok {
			return fmt.Errorf("系统代理快照缺少 %s", old.Kind)
		}
		alreadyRestored := now.Host == old.Host && now.Port == old.Port
		owned := now.Host == "127.0.0.1" && (now.Port == port || now.Port == old.Port)
		if !owned && !alreadyRestored {
			allUnchangedOrOwned = false
		}
		if !owned {
			continue
		}
		schema := "org.gnome.system.proxy." + old.Kind
		if _, e = run("gsettings", "set", schema, "host", old.Host); e != nil {
			return e
		}
		if _, e = run("gsettings", "set", schema, "port", strconv.Itoa(old.Port)); e != nil {
			return e
		}
	}
	if mode == "manual" && allUnchangedOrOwned {
		for _, x := range s.Entries {
			if x.Kind == "mode" {
				_, e = run("gsettings", "set", "org.gnome.system.proxy", "mode", strings.Trim(x.Raw, "'"))
				return e
			}
		}
	}
	return nil
}
