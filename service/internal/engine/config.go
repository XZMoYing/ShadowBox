package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"shadowbox/service/internal/profile"
)

func Resolve(id string, nodes []profile.Node) ([]profile.Node, error) {
	byID := map[string]profile.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	visiting := map[string]bool{}
	var out []profile.Node
	var walk func(string) error
	walk = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("代理链存在循环引用")
		}
		n, ok := byID[id]
		if !ok {
			return fmt.Errorf("代理链引用的节点已不存在")
		}
		visiting[id] = true
		defer delete(visiting, id)
		if len(n.Chain) > 0 {
			for _, child := range n.Chain {
				if e := walk(child); e != nil {
					return e
				}
			}
		} else {
			out = append(out, n)
		}
		if len(out) > 8 {
			return fmt.Errorf("代理链最多支持 8 跳")
		}
		return nil
	}
	if e := walk(id); e != nil {
		return nil, e
	}
	for i, n := range out {
		if i > 0 {
			if n.External != nil {
				return nil, fmt.Errorf("外部插件只能作为代理链入口")
			}
			switch n.Type() {
			case "hysteria", "hysteria2", "tuic", "wireguard":
				return nil, fmt.Errorf("%s 只支持作为代理链入口", n.Type())
			}
		}
	}
	return out, nil
}
func Compile(chain []profile.Node, s profile.Settings, externalPorts map[string]int) ([]byte, error) {
	if e := s.Validate(); e != nil {
		return nil, e
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("请先选择节点")
	}
	outs := []map[string]any{}
	previous := ""
	for i, n := range chain {
		o := profile.CloneMap(n.Outbound)
		if n.External != nil {
			port := externalPorts[n.ID]
			if port < 1 {
				return nil, fmt.Errorf("%s 缺少插件实例", n.Name)
			}
			o = map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": port, "version": "5"}
		}
		if o == nil {
			return nil, fmt.Errorf("节点没有出站配置")
		}
		tag := fmt.Sprintf("hop-%d", i)
		if i == len(chain)-1 {
			tag = "proxy"
		}
		o["tag"] = tag
		if previous != "" {
			o["detour"] = previous
		}
		previous = tag
		outs = append(outs, o)
	}
	outs = append(outs, map[string]any{"type": "direct", "tag": "direct"})
	inbounds := []map[string]any{{"type": "mixed", "tag": "mixed", "listen": "127.0.0.1", "listen_port": s.Port}}
	if s.Capture == "tun" {
		addresses := []string{"172.30.73.1/30"}
		if s.IPv6 {
			addresses = append(addresses, "fdfe:dcba:9876::1/126")
		}
		excludes := []string{}
		for _, n := range chain {
			server, _ := n.Outbound["server"].(string)
			if server == "" && n.External != nil {
				server, _ = n.External.Config["server"].(string)
			}
			if ip := net.ParseIP(server); ip != nil {
				bits := "/32"
				if ip.To4() == nil {
					bits = "/128"
				}
				excludes = append(excludes, server+bits)
			}
		}
		tun := map[string]any{"type": "tun", "tag": "tun", "address": addresses, "auto_route": true, "strict_route": true, "stack": "mixed", "mtu": 1500}
		if len(excludes) > 0 {
			tun["route_exclude_address"] = excludes
		}
		inbounds = append(inbounds, tun)
	}
	rules := []map[string]any{{"action": "sniff"}, {"protocol": "dns", "action": "hijack-dns"}}
	if s.Routing == "rule" {
		rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	}
	if s.Routing == "rule" {
		if len(s.DirectDomains) > 0 {
			rules = append(rules, map[string]any{"domain_suffix": s.DirectDomains, "outbound": "direct"})
		}
		if len(s.ProxyDomains) > 0 {
			rules = append(rules, map[string]any{"domain_suffix": s.ProxyDomains, "outbound": "proxy"})
		}
	}
	final := "proxy"
	if s.Routing == "direct" {
		final = "direct"
	}
	dnsServer := s.DNS
	dnsType := "tcp"
	dns := map[string]any{"type": dnsType, "tag": "remote", "server": dnsServer, "server_port": 53, "detour": final}
	if strings.HasPrefix(dnsServer, "https://") {
		return nil, fmt.Errorf("此版本 DNS 请输入 IP 地址，例如 1.1.1.1")
	}
	if net.ParseIP(dnsServer) == nil {
		return nil, fmt.Errorf("DNS 服务器必须为 IP 地址")
	}
	strategy := "prefer_ipv4"
	if !s.IPv6 {
		strategy = "ipv4_only"
	}
	cfg := map[string]any{"log": map[string]any{"level": "warn"}, "dns": map[string]any{"servers": []map[string]any{{"type": "local", "tag": "bootstrap"}, dns}, "final": "remote", "strategy": strategy}, "inbounds": inbounds, "outbounds": outs, "route": map[string]any{"rules": rules, "final": final, "auto_detect_interface": true, "default_domain_resolver": map[string]any{"server": "bootstrap", "strategy": strategy}}, "experimental": map[string]any{"clash_api": map[string]any{}}}
	return json.Marshal(cfg)
}
func ValidatePrivileged(data []byte) error {
	var v any
	if e := json.Unmarshal(data, &v); e != nil {
		return e
	}
	var walk func(any) error
	walk = func(v any) error {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				if strings.HasSuffix(k, "_path") || strings.HasSuffix(k, "_directory") || k == "output" || k == "external_ui" || k == "services" || k == "rule_set" || k == "include" {
					return fmt.Errorf("TUN 配置不允许读取本地文件或启动额外服务：%s", k)
				}
				if e := walk(v); e != nil {
					return e
				}
			}
		case []any:
			for _, v := range x {
				if e := walk(v); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return walk(v)
}
