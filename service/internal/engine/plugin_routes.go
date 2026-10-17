package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"shadowbox/service/internal/profile"
	"strings"
	"time"
)

// External executables cannot use sing-box's automatic socket binding. Resolve
// their upstreams before enabling TUN and exclude those IPs from its routes.
func ProtectPluginRoutes(data []byte, chain []profile.Node) ([]byte, error) {
	hosts := []string{}
	for _, n := range chain {
		if n.External == nil {
			continue
		}
		c := n.External.Config
		switch n.External.Kind {
		case "naive":
			s, _ := c["proxy"].(string)
			u, e := url.Parse(strings.TrimPrefix(s, "naive+"))
			if e != nil {
				return nil, e
			}
			hosts = append(hosts, u.Hostname())
		case "trojan-go":
			s, _ := c["remote_addr"].(string)
			hosts = append(hosts, s)
		case "mieru":
			b, _ := json.Marshal(c)
			var cfg struct {
				Profiles []struct {
					Servers []struct {
						IP     string `json:"ipAddress"`
						Domain string `json:"domainName"`
					} `json:"servers"`
				} `json:"profiles"`
			}
			_ = json.Unmarshal(b, &cfg)
			for _, p := range cfg.Profiles {
				for _, s := range p.Servers {
					if s.IP != "" {
						hosts = append(hosts, s.IP)
					} else {
						hosts = append(hosts, s.Domain)
					}
				}
			}
		}
	}
	if len(hosts) == 0 {
		return data, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	excludes := []any{}
	for _, host := range hosts {
		if host == "" {
			return nil, fmt.Errorf("插件节点缺少服务器地址")
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, fmt.Errorf("TUN 启动前解析插件服务器失败：%s", host)
		}
		for _, ip := range ips {
			suffix := "/128"
			if ip.IP.To4() != nil {
				suffix = "/32"
			}
			excludes = append(excludes, ip.IP.String()+suffix)
		}
	}
	var cfg map[string]any
	if e := json.Unmarshal(data, &cfg); e != nil {
		return nil, e
	}
	if a, ok := cfg["inbounds"].([]any); ok {
		for _, v := range a {
			m, ok := v.(map[string]any)
			if !ok || m["type"] != "tun" {
				continue
			}
			if old, ok := m["route_exclude_address"].([]any); ok {
				excludes = append(old, excludes...)
			}
			m["route_exclude_address"] = excludes
		}
	}
	return json.Marshal(cfg)
}
