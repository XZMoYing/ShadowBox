package profile

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ImportResult struct {
	Nodes  []Node   `json:"nodes"`
	Errors []string `json:"errors"`
}

func decode64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, e := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, err := e.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("Base64 编码无效")
}
func Parse(text string) ImportResult { return parse(text, 0) }
func parse(text string, depth int) ImportResult {
	r := ImportResult{Nodes: []Node{}, Errors: []string{}}
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		r.Errors = append(r.Errors, "没有找到节点内容")
		return r
	}
	if depth > 2 || len(text) > 16<<20 {
		r.Errors = append(r.Errors, "导入内容过大或嵌套过深")
		return r
	}
	if strings.HasPrefix(text, "[Interface]") {
		n, e := wireguard(text)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		} else {
			r.Nodes = append(r.Nodes, n)
		}
		return r
	}
	var obj any
	if json.Unmarshal([]byte(text), &obj) == nil {
		switch v := obj.(type) {
		case map[string]any:
			if a, ok := v["outbounds"].([]any); ok {
				for _, x := range a {
					if m, ok := x.(map[string]any); ok {
						r.addOutbound(m)
					}
				}
				return r
			}
			if a, ok := v["nodes"].([]any); ok {
				ids := map[string]string{}
				for _, x := range a {
					b, _ := json.Marshal(x)
					var n Node
					if json.Unmarshal(b, &n) == nil {
						old := n.ID
						n.ID = NewID()
						if old != "" {
							ids[old] = n.ID
						}
						n.Source = ""
						r.Nodes = append(r.Nodes, n)
					}
				}
				for i := range r.Nodes {
					for j, id := range r.Nodes[i].Chain {
						if next, ok := ids[id]; ok {
							r.Nodes[i].Chain[j] = next
						}
					}
				}
				return r
			}
			if _, ok := v["type"]; ok {
				r.addOutbound(v)
				return r
			}
		case []any:
			for _, x := range v {
				if m, ok := x.(map[string]any); ok {
					r.addOutbound(m)
				}
			}
			return r
		}
	}
	var y struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if yaml.Unmarshal([]byte(text), &y) == nil && len(y.Proxies) > 0 {
		for _, m := range y.Proxies {
			n, e := clash(m)
			if e != nil {
				r.Errors = append(r.Errors, fmt.Sprintf("%v: %v", m["name"], e))
			} else {
				r.Nodes = append(r.Nodes, n)
			}
		}
		return r
	}
	if !strings.Contains(text, "://") {
		if b, e := decode64(text); e == nil && string(b) != text {
			return parse(string(b), depth+1)
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for i := 0; scanner.Scan(); i++ {
		if i >= 10000 || len(r.Errors) >= 100 {
			r.Errors = append(r.Errors, "内容过多，最多解析 10000 行和报告 100 条错误")
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, e := parseURI(line)
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("第 %d 行：%v", i+1, e))
		} else {
			r.Nodes = append(r.Nodes, n)
		}
	}
	return r
}
func ParseZip(data []byte) ImportResult {
	r := ImportResult{Nodes: []Node{}, Errors: []string{}}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		r.Errors = append(r.Errors, e.Error())
		return r
	}
	var total int64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if len(r.Nodes) > 10000 || len(z.File) > 100 || f.UncompressedSize64 > 16<<20 {
			r.Errors = append(r.Errors, "ZIP 文件数量或解压大小超过限制")
			break
		}
		c, e := f.Open()
		if e != nil {
			continue
		}
		b, e := io.ReadAll(io.LimitReader(c, 16<<20))
		c.Close()
		total += int64(len(b))
		if total > 32<<20 {
			r.Errors = append(r.Errors, "ZIP 内容超过 32 MiB")
			break
		}
		if e == nil {
			p := Parse(string(b))
			r.Nodes = append(r.Nodes, p.Nodes...)
			for _, s := range p.Errors {
				r.Errors = append(r.Errors, f.Name+": "+s)
			}
		}
	}
	return r
}
func node(name string, o map[string]any) Node {
	if name == "" {
		name = fmt.Sprintf("%v:%v", o["server"], o["server_port"])
	}
	return Node{ID: NewID(), Name: name, Group: "本地节点", Outbound: o, Latency: -1, UpdatedAt: time.Now()}
}
func (r *ImportResult) addOutbound(m map[string]any) {
	t := str(m["type"])
	switch t {
	case "direct", "block", "dns", "selector", "urltest":
		return
	default:
		if t == "" {
			r.Errors = append(r.Errors, "出站缺少 type")
			return
		}
		name := str(m["tag"])
		o := CloneMap(m)
		delete(o, "tag")
		r.Nodes = append(r.Nodes, node(name, o))
	}
}
func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func integer(v any) int {
	n, _ := strconv.Atoi(str(v))
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return n
}
func truth(v any) bool { s := strings.ToLower(str(v)); return s == "true" || s == "1" }
func list(v any) []string {
	if a, ok := v.([]any); ok {
		r := []string{}
		for _, x := range a {
			r = append(r, str(x))
		}
		return r
	}
	if s := str(v); s != "" {
		return strings.Split(s, ",")
	}
	return nil
}
func parseURI(raw string) (Node, error) {
	if strings.HasPrefix(raw, "sn://") {
		return neko(raw)
	}
	if strings.HasPrefix(raw, "vmess://") {
		b, e := decode64(strings.TrimPrefix(raw, "vmess://"))
		if e == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				return vmess(m)
			}
		}
	}
	if strings.HasPrefix(raw, "ss://") {
		return shadowsocks(raw)
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" {
		return Node{}, fmt.Errorf("无效或不支持的分享链接")
	}
	t := strings.ToLower(u.Scheme)
	q := u.Query()
	p := 443
	if u.Port() != "" {
		p, e = strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return Node{}, fmt.Errorf("服务器端口无效")
		}
	}
	name := u.Fragment
	o := map[string]any{"type": t, "server": u.Hostname(), "server_port": p}
	user := ""
	pass := ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	switch t {
	case "vless":
		o["uuid"] = user
		if q.Get("flow") != "" {
			o["flow"] = q.Get("flow")
		}
		if q.Get("packetEncoding") != "" {
			o["packet_encoding"] = q.Get("packetEncoding")
		}
	case "trojan", "anytls":
		o["password"] = user
	case "socks", "socks5", "socks4", "socks4a":
		o["type"] = "socks"
		o["version"] = "5"
		if t == "socks4" {
			o["version"] = "4"
		}
		if t == "socks4a" {
			o["version"] = "4a"
		}
		if user != "" {
			o["username"] = user
			o["password"] = pass
		}
		if u.Port() == "" {
			o["server_port"] = 1080
		}
	case "http", "https":
		o["type"] = "http"
		if user != "" {
			o["username"] = user
			o["password"] = pass
		}
		if t == "http" && u.Port() == "" {
			o["server_port"] = 80
		}
	case "hy2", "hysteria2":
		o["type"] = "hysteria2"
		if pass != "" {
			user += ":" + pass
		}
		o["password"] = user
		if q.Get("obfs") == "salamander" {
			o["obfs"] = map[string]any{"type": "salamander", "password": q.Get("obfs-password")}
		}
		if q.Get("mport") != "" {
			ranges, err := portRanges(q.Get("mport"))
			if err != nil {
				return Node{}, err
			}
			o["server_ports"] = ranges
		}
	case "hysteria":
		o["auth_str"] = q.Get("auth")
		if o["auth_str"] == "" {
			o["auth_str"] = user
		}
		o["up_mbps"] = integer(q.Get("upmbps"))
		o["down_mbps"] = integer(q.Get("downmbps"))
		if q.Get("obfsParam") != "" {
			o["obfs"] = q.Get("obfsParam")
		}
	case "tuic":
		o["uuid"] = user
		o["password"] = pass
		if q.Get("congestion_control") != "" {
			o["congestion_control"] = q.Get("congestion_control")
		}
		if q.Get("udp_relay_mode") != "" {
			o["udp_relay_mode"] = q.Get("udp_relay_mode")
		}
	case "ssh":
		o["user"] = user
		o["password"] = pass
		if u.Port() == "" {
			o["server_port"] = 22
		}
	case "trojan-go":
		if user == "" {
			return Node{}, fmt.Errorf("Trojan-Go 密码不能为空")
		}
		if q.Get("plugin") != "" {
			return Node{}, fmt.Errorf("Trojan-Go 额外 SIP003 插件暂不支持")
		}
		cfg, err := trojanGoConfig(u.Hostname(), p, user, q.Get("sni"), q.Get("type"), q.Get("host"), q.Get("path"), q.Get("encryption"), truth(q.Get("allowInsecure")))
		if err != nil {
			return Node{}, err
		}
		n := node(defaultName(name, u.Host), nil)
		n.External = &External{Kind: "trojan-go", Config: cfg}
		return n, nil
	case "naive+https", "naive+http":
		return Node{ID: NewID(), Name: defaultName(name, u.Host), Group: "本地节点", External: &External{Kind: "naive", Config: map[string]any{"proxy": rawWithoutFragment(raw)}}, Latency: -1, UpdatedAt: time.Now()}, nil
	default:
		return Node{}, fmt.Errorf("暂不能解析 %s；可通过 sing-box 出站 JSON 导入受内核支持的协议", t)
	}
	if (t == "vless" || t == "trojan" || t == "anytls" || t == "tuic" || t == "hy2" || t == "hysteria2") && user == "" {
		return Node{}, fmt.Errorf("节点缺少 UUID 或密码")
	}
	security := q.Get("security")
	if security == "tls" || security == "reality" || t == "trojan" || t == "anytls" || t == "https" || t == "hy2" || t == "hysteria2" || t == "hysteria" || t == "tuic" {
		tls := map[string]any{"enabled": true}
		sni := q.Get("sni")
		if sni == "" {
			sni = q.Get("peer")
		}
		if sni != "" {
			tls["server_name"] = sni
		}
		if truth(q.Get("insecure")) || truth(q.Get("allowInsecure")) {
			tls["insecure"] = true
		}
		if alpn := q.Get("alpn"); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}
		if fp := q.Get("fp"); fp != "" || security == "reality" {
			if fp == "" {
				fp = "chrome"
			}
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if security == "reality" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": q.Get("pbk"), "short_id": q.Get("sid")}
		}
		o["tls"] = tls
	}
	if e := transport(o, q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName")); e != nil {
		return Node{}, e
	}
	return node(name, o), nil
}
func defaultName(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
func rawWithoutFragment(s string) string { return strings.SplitN(s, "#", 2)[0] }
func shadowsocks(raw string) (Node, error) {
	body := strings.TrimPrefix(raw, "ss://")
	main := strings.SplitN(body, "#", 2)[0]
	name := ""
	if a := strings.SplitN(body, "#", 2); len(a) == 2 {
		name, _ = url.PathUnescape(a[1])
	}
	if !strings.Contains(main, "@") {
		b, e := decode64(strings.SplitN(main, "?", 2)[0])
		if e != nil {
			return Node{}, e
		}
		raw = "ss://" + string(b)
		if name != "" {
			raw += "#" + url.PathEscape(name)
		}
	}
	u, e := url.Parse(raw)
	if e != nil || u.User == nil {
		return Node{}, fmt.Errorf("Shadowsocks 链接无效")
	}
	auth := u.User.String()
	if !strings.Contains(auth, ":") {
		b, e := decode64(auth)
		if e != nil {
			return Node{}, e
		}
		auth = string(b)
	} else {
		auth, _ = url.PathUnescape(auth)
	}
	method, pass, ok := strings.Cut(auth, ":")
	p, _ := strconv.Atoi(u.Port())
	if !ok || p < 1 || p > 65535 || u.Hostname() == "" {
		return Node{}, fmt.Errorf("Shadowsocks 参数不完整")
	}
	o := map[string]any{"type": "shadowsocks", "server": u.Hostname(), "server_port": p, "method": method, "password": pass}
	if plugin := u.Query().Get("plugin"); plugin != "" {
		a := strings.SplitN(plugin, ";", 2)
		o["plugin"] = a[0]
		if len(a) > 1 {
			o["plugin_opts"] = a[1]
		}
	}
	return node(defaultName(u.Fragment, name), o), nil
}
func vmess(m map[string]any) (Node, error) {
	o := map[string]any{"type": "vmess", "server": str(m["add"]), "server_port": integer(m["port"]), "uuid": str(m["id"]), "security": defaultName(str(m["scy"]), "auto"), "alter_id": integer(m["aid"])}
	if truth(m["tls"]) || str(m["tls"]) == "tls" {
		tls := map[string]any{"enabled": true, "server_name": defaultName(str(m["sni"]), str(m["host"]))}
		if a := list(m["alpn"]); len(a) > 0 {
			tls["alpn"] = a
		}
		if truth(m["allowInsecure"]) {
			tls["insecure"] = true
		}
		if fp := str(m["fp"]); fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		o["tls"] = tls
	}
	if e := transport(o, str(m["net"]), str(m["host"]), str(m["path"]), str(m["path"])); e != nil {
		return Node{}, e
	}
	return node(str(m["ps"]), o), nil
}
func transport(o map[string]any, t, host, path, service string) error {
	switch t {
	case "", "tcp", "none":
	case "ws":
		x := map[string]any{"type": "ws", "path": path}
		if host != "" {
			x["headers"] = map[string]any{"Host": host}
		}
		o["transport"] = x
	case "grpc":
		o["transport"] = map[string]any{"type": "grpc", "service_name": service}
	case "http", "h2":
		x := map[string]any{"type": "http", "path": path}
		if host != "" {
			x["host"] = strings.Split(host, ",")
		}
		o["transport"] = x
	case "httpupgrade":
		o["transport"] = map[string]any{"type": "httpupgrade", "host": host, "path": path}
	case "quic":
		o["transport"] = map[string]any{"type": "quic"}
	default:
		return fmt.Errorf("不支持的传输方式 %s", t)
	}
	return nil
}
func wireguard(s string) (Node, error) {
	var private, public, endpoint, preshared string
	addresses := []string{}
	for _, l := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "privatekey":
			private = strings.TrimSpace(v)
		case "publickey":
			public = strings.TrimSpace(v)
		case "endpoint":
			endpoint = strings.TrimSpace(v)
		case "presharedkey":
			preshared = strings.TrimSpace(v)
		case "address":
			for _, a := range strings.Split(v, ",") {
				addresses = append(addresses, strings.TrimSpace(a))
			}
		}
	}
	h, p, e := net.SplitHostPort(endpoint)
	if e != nil || private == "" || public == "" {
		return Node{}, fmt.Errorf("WireGuard 配置需要 PrivateKey、PublicKey 和 Endpoint")
	}
	port, _ := strconv.Atoi(p)
	o := map[string]any{"type": "wireguard", "server": h, "server_port": port, "private_key": private, "peer_public_key": public, "local_address": addresses}
	if preshared != "" {
		o["pre_shared_key"] = preshared
	}
	return node("WireGuard "+h, o), nil
}
