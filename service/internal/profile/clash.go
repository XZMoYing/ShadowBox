package profile

import "fmt"

func clash(m map[string]any) (Node, error) {
	t := str(m["type"])
	o := map[string]any{"type": t, "server": str(m["server"]), "server_port": integer(m["port"])}
	switch t {
	case "ss":
		o["type"] = "shadowsocks"
		o["method"] = str(m["cipher"])
		o["password"] = str(m["password"])
		if p := str(m["plugin"]); p != "" {
			o["plugin"] = p
			if a, ok := m["plugin-opts"].(map[string]any); ok {
				opts := ""
				for k, v := range a {
					if opts != "" {
						opts += ";"
					}
					if truth(v) {
						opts += k
					} else {
						opts += k + "=" + str(v)
					}
				}
				o["plugin_opts"] = opts
			}
		}
	case "vmess":
		o["uuid"] = str(m["uuid"])
		o["alter_id"] = integer(m["alterId"])
		o["security"] = defaultName(str(m["cipher"]), "auto")
	case "vless":
		o["uuid"] = str(m["uuid"])
		if v := str(m["flow"]); v != "" {
			o["flow"] = v
		}
		if p := str(m["packet-encoding"]); p != "" {
			o["packet_encoding"] = p
		}
	case "trojan", "anytls":
		o["password"] = str(m["password"])
	case "http", "socks5":
		if t == "socks5" {
			o["type"] = "socks"
			o["version"] = "5"
		}
		if str(m["username"]) != "" {
			o["username"] = str(m["username"])
			o["password"] = str(m["password"])
		}
	case "hysteria2":
		o["password"] = str(m["password"])
		if raw := str(m["ports"]); raw != "" {
			ranges, e := portRanges(raw)
			if e != nil {
				return Node{}, e
			}
			o["server_ports"] = ranges
		}
		if hop := integer(m["hop-interval"]); hop > 0 {
			o["hop_interval"] = fmt.Sprintf("%ds", hop)
		}
		if str(m["obfs"]) != "" {
			o["obfs"] = map[string]any{"type": str(m["obfs"]), "password": str(m["obfs-password"])}
		}
	case "hysteria":
		o["auth_str"] = defaultName(str(m["auth-str"]), str(m["auth"]))
		o["up_mbps"] = integer(m["up"])
		o["down_mbps"] = integer(m["down"])
		if str(m["obfs"]) != "" {
			o["obfs"] = str(m["obfs"])
		}
	case "tuic":
		o["uuid"] = str(m["uuid"])
		o["password"] = str(m["password"])
		if v := str(m["congestion-controller"]); v != "" {
			o["congestion_control"] = v
		}
		if v := str(m["udp-relay-mode"]); v != "" {
			o["udp_relay_mode"] = v
		}
	case "wireguard":
		o["private_key"] = str(m["private-key"])
		o["peer_public_key"] = str(m["public-key"])
		a := []string{}
		if v := str(m["ip"]); v != "" {
			a = append(a, v+"/32")
		}
		if v := str(m["ipv6"]); v != "" {
			a = append(a, v+"/128")
		}
		o["local_address"] = a
		if v := str(m["pre-shared-key"]); v != "" {
			o["pre_shared_key"] = v
		}
		if m["reserved"] != nil {
			o["reserved"] = m["reserved"]
		}
	case "ssh":
		o["user"] = str(m["username"])
		o["password"] = str(m["password"])
	case "shadow-tls":
		o["type"] = "shadowtls"
		o["version"] = integer(m["version"])
		o["password"] = str(m["password"])
	case "direct", "reject":
		return Node{}, fmt.Errorf("不是代理节点")
	default:
		return Node{}, fmt.Errorf("未知 Clash 协议 %s，请使用 sing-box 出站 JSON", t)
	}
	if truth(m["tls"]) || t == "trojan" || t == "anytls" || t == "tuic" || t == "hysteria" || t == "hysteria2" || t == "shadow-tls" {
		tls := map[string]any{"enabled": true}
		if sni := defaultName(str(m["servername"]), str(m["sni"])); sni != "" {
			tls["server_name"] = sni
		}
		if truth(m["skip-cert-verify"]) {
			tls["insecure"] = true
		}
		if a := list(m["alpn"]); len(a) > 0 {
			tls["alpn"] = a
		}
		fp := str(m["client-fingerprint"])
		if fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if re, ok := m["reality-opts"].(map[string]any); ok {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": defaultName(fp, "chrome")}
			tls["reality"] = map[string]any{"enabled": true, "public_key": str(re["public-key"]), "short_id": str(re["short-id"])}
		}
		o["tls"] = tls
	}
	network := str(m["network"])
	host, path, service := "", "", ""
	if x, ok := m["ws-opts"].(map[string]any); ok {
		path = str(x["path"])
		if h, ok := x["headers"].(map[string]any); ok {
			host = defaultName(str(h["Host"]), str(h["host"]))
		}
	}
	if x, ok := m["grpc-opts"].(map[string]any); ok {
		service = str(x["grpc-service-name"])
	}
	if x, ok := m["h2-opts"].(map[string]any); ok {
		path = str(x["path"])
		a := list(x["host"])
		if len(a) > 0 {
			host = a[0]
		}
	}
	if e := transport(o, network, host, path, service); e != nil {
		return Node{}, e
	}
	if integer(m["port"]) < 1 || str(m["server"]) == "" {
		return Node{}, fmt.Errorf("缺少服务器或端口")
	}
	return node(str(m["name"]), o), nil
}
