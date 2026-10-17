package profile

// Decoder for NekoBox 1.4.2's sn:// format. Field ordering is derived from
// AbstractBean and the protocol beans retained in third_party/nekobox-reference.
// Kryo 5 uses fixed little-endian ints and a flagged string length (UTF-16 units).
import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
)

type nekoReader struct {
	b   []byte
	p   int
	err error
}

func (r *nekoReader) fail(s string) {
	if r.err == nil {
		r.err = fmt.Errorf("sn:// %s（偏移 %d）", s, r.p)
	}
}
func (r *nekoReader) byte() byte {
	if r.p >= len(r.b) {
		r.fail("内容截断")
		return 0
	}
	v := r.b[r.p]
	r.p++
	return v
}
func (r *nekoReader) int() int {
	if len(r.b)-r.p < 4 {
		r.fail("整数截断")
		return 0
	}
	v := int(int32(binary.LittleEndian.Uint32(r.b[r.p:])))
	r.p += 4
	return v
}
func (r *nekoReader) bool() bool {
	v := r.byte()
	if v > 1 {
		r.fail("布尔值无效")
	}
	return v == 1
}
func (r *nekoReader) version(want int) {
	if v := r.int(); v != want {
		r.fail(fmt.Sprintf("版本 %d 暂不支持，需要 %d", v, want))
	}
}
func (r *nekoReader) str() string {
	if r.err != nil {
		return ""
	}
	b := r.byte()
	if b&128 == 0 {
		out := []byte{b}
		for r.err == nil && len(out) <= 1<<20 {
			b = r.byte()
			out = append(out, b&127)
			if b&128 != 0 {
				return string(out)
			}
		}
		r.fail("ASCII 字符串无效")
		return ""
	}
	count := int(b & 63)
	if b&64 != 0 {
		for shift := 6; shift <= 27; shift += 7 {
			b = r.byte()
			count |= int(b&127) << shift
			if b&128 == 0 {
				break
			}
			if shift == 27 {
				r.fail("字符串长度无效")
			}
		}
	}
	if count <= 1 {
		return ""
	}
	count--
	if count > 1<<20 {
		r.fail("字符串过大")
		return ""
	}
	chars := make([]uint16, 0, count)
	for i := 0; i < count && r.err == nil; i++ {
		b = r.byte()
		var c uint16
		switch {
		case b < 128:
			c = uint16(b)
		case b&224 == 192:
			c = uint16(b&31)<<6 | uint16(r.byte()&63)
		case b&240 == 224:
			c = uint16(b&15)<<12 | uint16(r.byte()&63)<<6 | uint16(r.byte()&63)
		default:
			r.fail("UTF-8 字符无效")
		}
		chars = append(chars, c)
	}
	return string(utf16.Decode(chars))
}
func splitLines(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
}
func nekoTLS(sni, alpn, cert, fp string, insecure bool) map[string]any {
	t := map[string]any{"enabled": true, "insecure": insecure}
	if sni != "" {
		t["server_name"] = sni
	}
	if alpn != "" {
		t["alpn"] = splitLines(alpn)
	}
	if cert != "" {
		t["certificate"] = []string{cert}
	}
	if fp != "" {
		t["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	return t
}
func nekoECH(t map[string]any, enabled bool, config string) {
	if enabled {
		t["ech"] = map[string]any{"enabled": true, "config": []string{config}}
	}
}
func (r *nekoReader) base(t string) map[string]any {
	return map[string]any{"type": t, "server": strings.Trim(r.str(), "[]"), "server_port": r.int()}
}
func (r *nekoReader) v2ray(t string) map[string]any {
	r.version(4)
	o := r.base(t)
	uuid, enc := r.str(), r.str()
	if t == "vmess" {
		alter := r.int()
		if alter == -1 {
			o["type"] = "vless"
			o["uuid"] = uuid
			if enc != "" && enc != "none" {
				o["flow"] = enc
			}
		} else {
			o["uuid"] = uuid
			o["security"] = defaultName(enc, "auto")
			o["alter_id"] = alter
		}
	}
	transportType := r.str()
	host, path, header := "", "", ""
	early := 0
	switch transportType {
	case "tcp", "quic":
	case "ws":
		host, path = r.str(), r.str()
		early, header = r.int(), r.str()
	case "http", "httpupgrade":
		host, path = r.str(), r.str()
	case "grpc":
		path = r.str()
	default:
		r.fail("传输类型不支持")
	}
	if err := transport(o, transportType, host, path, path); err != nil {
		r.fail(err.Error())
	}
	if transportType == "ws" {
		tr, _ := o["transport"].(map[string]any)
		if early > 0 {
			tr["max_early_data"] = early
			tr["early_data_header_name"] = header
		}
	}
	sec := r.str()
	tls := map[string]any{}
	if sec == "tls" {
		sni, alpn, cert := r.str(), r.str(), r.str()
		insecure := r.bool()
		fp, pub, sid := r.str(), r.str(), r.str()
		tls = nekoTLS(sni, alpn, cert, fp, insecure)
		if pub != "" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": pub, "short_id": sid}
		}
	}
	ech, echCfg := r.bool(), r.str()
	if ech && sec != "tls" {
		r.fail("ECH 缺少 TLS")
	}
	nekoECH(tls, ech, echCfg)
	if sec == "tls" {
		o["tls"] = tls
	}
	packet := r.int()
	if packet == 1 {
		o["packet_encoding"] = "packetaddr"
	} else if packet == 2 {
		o["packet_encoding"] = "xudp"
	}
	mux, pad, kind, count := r.bool(), r.bool(), r.int(), r.int()
	if mux {
		protocol := []string{"h2mux", "smux", "yamux"}
		if kind < 0 || kind >= len(protocol) {
			r.fail("Mux 类型不支持")
		} else {
			o["multiplex"] = map[string]any{"enabled": true, "padding": pad, "protocol": protocol[kind], "max_streams": count}
		}
	}
	return o
}
func neko(raw string) (Node, error) {
	body := strings.TrimPrefix(raw, "sn://")
	kind, data, compressed := "", "", true
	if k, d, ok := strings.Cut(body, "?"); ok {
		kind, data = k, d
	} else if k, d, ok := strings.Cut(body, ":"); ok {
		kind, data, compressed = k, d, false
	} else {
		return Node{}, fmt.Errorf("sn:// 分享格式无效")
	}
	b, e := decode64(data)
	if e != nil {
		return Node{}, e
	}
	if compressed {
		z, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return Node{}, fmt.Errorf("sn:// 压缩数据无效")
		}
		b, e = io.ReadAll(io.LimitReader(z, (1<<20)+1))
		z.Close()
		if e != nil {
			return Node{}, e
		}
	}
	if len(b) > 1<<20 {
		return Node{}, fmt.Errorf("sn:// 数据超过 1 MiB")
	}
	r := &nekoReader{b: b}
	var o map[string]any
	var external *External
	switch kind {
	case "ss":
		r.version(2)
		o = r.base("shadowsocks")
		o["method"], o["password"] = r.str(), r.str()
		plugin := r.str()
		if plugin != "" {
			p, opts, _ := strings.Cut(plugin, ";")
			o["plugin"], o["plugin_opts"] = p, opts
		}
		if r.bool() {
			o["udp_over_tcp"] = true
		}
	case "socks":
		r.version(2)
		o = r.base("socks")
		v := r.int()
		if v < 0 || v > 2 {
			r.fail("SOCKS 版本无效")
		} else {
			o["version"] = []string{"4", "4a", "5"}[v]
		}
		o["username"], o["password"] = r.str(), r.str()
		if r.bool() {
			o["udp_over_tcp"] = true
		}
	case "vmess":
		o = r.v2ray("vmess")
	case "http":
		r.version(0)
		o = r.v2ray("http")
		o["username"], o["password"] = r.str(), r.str()
	case "trojan":
		r.version(2)
		o = r.v2ray("trojan")
		o["password"] = r.str()
	case "anytls":
		r.version(0)
		o = r.base("anytls")
		o["password"] = r.str()
		sni, alpn, cert, fp := r.str(), r.str(), r.str(), r.str()
		insecure, ech := r.bool(), r.str()
		tls := nekoTLS(sni, alpn, cert, fp, insecure)
		nekoECH(tls, ech != "", ech)
		o["tls"] = tls
	case "wg":
		r.version(2)
		o = r.base("wireguard")
		o["local_address"] = splitLines(r.str())
		o["private_key"], o["peer_public_key"], o["pre_shared_key"] = r.str(), r.str(), r.str()
		o["mtu"] = r.int()
		reserved := r.str()
		if reserved != "" {
			var a []int
			for _, s := range splitLines(reserved) {
				a = append(a, integer(strings.TrimSpace(s)))
			}
			o["reserved"] = a
		}
	case "ssh":
		r.version(0)
		o = r.base("ssh")
		o["user"] = r.str()
		switch r.int() {
		case 0:
		case 1:
			o["password"] = r.str()
		case 2:
			o["private_key"], o["private_key_passphrase"] = []string{r.str()}, r.str()
		default:
			r.fail("SSH 认证类型无效")
		}
		if pub := r.str(); pub != "" {
			o["host_key"] = splitLines(pub)
		}
	case "naive":
		r.version(3)
		base := r.base("")
		proto, user, pass, headers, cert, sni := r.str(), r.str(), r.str(), r.str(), r.str(), r.str()
		concurrency, uot := r.int(), r.bool()
		if cert != "" || sni != "" || uot {
			r.fail("Naive 自定义证书/SNI/UoT 暂不支持，请通过插件配置导入")
		}
		u := url.URL{Scheme: proto, Host: net.JoinHostPort(str(base["server"]), str(base["server_port"])), User: url.UserPassword(user, pass)}
		cfg := map[string]any{"proxy": u.String()}
		if headers != "" {
			cfg["extra-headers"] = headers
		}
		if concurrency > 0 {
			cfg["insecure-concurrency"] = concurrency
		}
		external = &External{Kind: "naive", Config: cfg}
	case "trojan-go":
		r.version(1)
		base := r.base("")
		password, sni, t := r.str(), r.str(), r.str()
		host, path := "", ""
		if t == "ws" {
			host, path = r.str(), r.str()
		}
		enc, plugin, insecure := r.str(), r.str(), r.bool()
		if plugin != "" {
			r.fail("Trojan-Go 额外 SIP003 插件暂不支持")
		}
		cfg, err := trojanGoConfig(str(base["server"]), integer(base["server_port"]), password, sni, t, host, path, enc, insecure)
		if err != nil {
			r.fail(err.Error())
		}
		external = &External{Kind: "trojan-go", Config: cfg}
	case "mieru":
		r.version(0)
		base := r.base("")
		protocol, user, password := r.str(), r.str(), r.str()
		mtu := 1400
		if protocol == "UDP" {
			mtu = r.int()
		}
		cfg := map[string]any{"activeProfile": "default", "loggingLevel": "INFO", "profiles": []any{map[string]any{"profileName": "default", "user": map[string]any{"name": user, "password": password}, "servers": []any{map[string]any{"ipAddress": base["server"], "portBindings": []any{map[string]any{"port": base["server_port"], "protocol": protocol}}}}, "mtu": mtu}}}
		external = &External{Kind: "mieru", Config: cfg}
	case "hysteria":
		r.version(7)
		o = r.base("hysteria")
		proto, authType, auth, transport, obfs, sni, alpn := r.int(), r.int(), r.str(), r.int(), r.str(), r.str(), r.str()
		up, down, insecure, cert := r.int(), r.int(), r.bool(), r.str()
		sw, cw, disable, hop, ports := r.int(), r.int(), r.bool(), r.int(), r.str()
		o["tls"] = nekoTLS(sni, alpn, cert, "", insecure)
		if up > 0 {
			o["up_mbps"] = up
		}
		if down > 0 {
			o["down_mbps"] = down
		}
		if proto == 2 {
			o["type"] = "hysteria2"
			o["password"] = auth
			if obfs != "" {
				o["obfs"] = map[string]any{"type": "salamander", "password": obfs}
			}
			if ports != "" {
				ranges, e := portRanges(ports)
				if e != nil {
					r.fail(e.Error())
				}
				o["server_ports"] = ranges
			}
			if hop > 0 {
				o["hop_interval"] = fmt.Sprintf("%ds", hop)
			}
		} else if proto == 1 {
			if transport != 0 {
				r.fail("Hysteria 1 非 UDP 传输暂不支持")
			}
			if ports != "" {
				ranges, e := portRanges(ports)
				if e != nil {
					r.fail(e.Error())
				}
				o["server_ports"] = ranges
			}
			if hop > 0 {
				o["hop_interval"] = fmt.Sprintf("%ds", hop)
			}
			key := "auth_str"
			if authType == 2 {
				key = "auth"
			}
			o[key] = auth
			o["obfs"] = obfs
			if sw > 0 {
				o["recv_window_conn"] = sw
			}
			if cw > 0 {
				o["recv_window"] = cw
			}
			o["disable_mtu_discovery"] = disable
		} else {
			r.fail("Hysteria 版本无效")
		}
	case "tuic":
		r.version(2)
		o = r.base("tuic")
		token, cert, relay, congestion, alpn := r.str(), r.str(), r.str(), r.str(), r.str()
		disableSNI, reduceRTT, mtu, sni, fast, insecure, custom, proto, uuid := r.bool(), r.bool(), r.int(), r.str(), r.bool(), r.bool(), r.str(), r.int(), r.str()
		_, _, _ = reduceRTT, mtu, custom
		if proto != 5 {
			r.fail("TUIC v4 已不受同款内核支持")
		}
		if custom != "" {
			r.fail("TUIC 额外 JSON 需要手动迁移")
		}
		o["uuid"], o["password"], o["udp_relay_mode"], o["congestion_control"], o["zero_rtt_handshake"] = uuid, token, relay, congestion, fast
		tls := nekoTLS(sni, alpn, cert, "", insecure)
		tls["disable_sni"] = disableSNI
		o["tls"] = tls
	case "config":
		r.version(0)
		_ = r.base("")
		t, cfg := r.int(), r.str()
		if t != 1 {
			return Node{}, fmt.Errorf("sn:// 完整自定义配置暂不能自动迁移，请导入其中的出站 JSON")
		}
		if json.Unmarshal([]byte(cfg), &o) != nil {
			r.fail("自定义出站 JSON 无效")
		}
	default:
		return Node{}, fmt.Errorf("sn:// 类型 %s 暂不支持", kind)
	}
	if r.err != nil {
		return Node{}, r.err
	}
	r.version(1)
	name, patch, custom := r.str(), r.str(), r.str()
	if r.err != nil {
		return Node{}, r.err
	}
	if r.p != len(b) {
		return Node{}, fmt.Errorf("sn:// 存在未知扩展字段")
	}
	if custom != "" {
		return Node{}, fmt.Errorf("sn:// 附带完整配置补丁，请手动迁移，避免丢失原有规则")
	}
	if patch != "" {
		if external != nil {
			return Node{}, fmt.Errorf("插件节点的出站补丁请手动迁移")
		}
		var m map[string]any
		if json.Unmarshal([]byte(patch), &m) != nil {
			return Node{}, fmt.Errorf("sn:// 出站补丁不是 JSON 对象")
		}
		mergeObject(o, m)
	}
	n := node(name, o)
	n.External = external
	if external != nil && name == "" {
		n.Name = external.Kind
	}
	return n, nil
}
func mergeObject(dst, src map[string]any) {
	for k, v := range src {
		if a, ok := dst[k].(map[string]any); ok {
			if b, ok := v.(map[string]any); ok {
				mergeObject(a, b)
				continue
			}
		}
		dst[k] = v
	}
}
func trojanGoConfig(server string, port int, password, sni, t, host, path, encryption string, insecure bool) (map[string]any, error) {
	cfg := map[string]any{"remote_addr": server, "remote_port": port, "password": []string{password}, "ssl": map[string]any{"sni": sni, "verify": !insecure}}
	if t == "ws" {
		cfg["websocket"] = map[string]any{"enabled": true, "host": host, "path": path}
	} else if t != "" && t != "original" {
		return nil, fmt.Errorf("Trojan-Go 传输类型不支持")
	}
	if encryption != "" && encryption != "none" {
		if !strings.HasPrefix(encryption, "ss;") {
			return nil, fmt.Errorf("Trojan-Go 加密参数无效")
		}
		method, password, ok := strings.Cut(strings.TrimPrefix(encryption, "ss;"), ":")
		if !ok {
			return nil, fmt.Errorf("Trojan-Go 加密参数不完整")
		}
		cfg["shadowsocks"] = map[string]any{"enabled": true, "method": method, "password": password}
	}
	return cfg, nil
}

func portRanges(raw string) ([]string, error) {
	var result []string
	for _, p := range splitLines(raw) {
		p = strings.TrimSpace(strings.ReplaceAll(p, "-", ":"))
		start, end, ok := strings.Cut(p, ":")
		if !ok {
			end = start
		}
		a, e1 := strconv.Atoi(start)
		b, e2 := strconv.Atoi(end)
		if e1 != nil || e2 != nil || a < 1 || b < a || b > 65535 {
			return nil, fmt.Errorf("端口范围无效：%s", p)
		}
		result = append(result, fmt.Sprintf("%d:%d", a, b))
	}
	return result, nil
}
