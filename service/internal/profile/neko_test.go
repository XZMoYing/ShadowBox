package profile

import (
	"os"
	"strings"
	"testing"
)

func TestNeko142KryoFixtures(t *testing.T) {
	b, e := os.ReadFile("testdata/neko-1.4.2.txt")
	if e != nil {
		t.Fatal(e)
	}
	result := Parse(string(b))
	if len(result.Errors) > 0 {
		t.Fatal(result.Errors)
	}
	if len(result.Nodes) != 10 {
		t.Fatalf("got %d", len(result.Nodes))
	}
	expected := []string{"shadowsocks", "socks", "vmess", "vless", "anytls", "wireguard", "hysteria2", "tuic", "mieru", "trojan-go"}
	for i, n := range result.Nodes {
		if n.Type() != expected[i] {
			t.Errorf("%d got %s", i, n.Type())
		}
		if !strings.HasPrefix(n.Name, "测试节点 🍃 ") {
			t.Errorf("UTF-16 decoding: %q", n.Name)
		}
	}
	if result.Nodes[0].Outbound["password"] != "test password:中文" {
		t.Fatal("password not preserved")
	}
	if result.Nodes[3].Outbound["flow"] != "xtls-rprx-vision" {
		t.Fatal("flow missing")
	}
	if got := result.Nodes[6].Outbound["server_ports"].([]string); got[0] != "443:443" || got[1] != "8443:8450" {
		t.Fatal(got)
	}
	if result.Nodes[6].Outbound["password"] != "hy2 password" {
		t.Fatal("Hysteria auth missing")
	}
	ws := result.Nodes[2].Outbound["transport"].(map[string]any)
	if ws["max_early_data"] != 2048 {
		t.Fatal("WS early data missing")
	}
	tg := result.Nodes[9].External.Config
	if tg["remote_addr"] != "example.com" || tg["websocket"] == nil || tg["shadowsocks"] == nil {
		t.Fatal("Trojan-Go options missing")
	}
}
func TestNekoRejectsTruncatedOrUnknownVersions(t *testing.T) {
	for _, s := range []string{"sn://ss?invalid", "sn://ss:AAAAAA", "sn://neko?eJwDAAAAAAE", "sn://ss:_____w"} {
		if r := Parse(s); len(r.Nodes) > 0 || len(r.Errors) == 0 {
			t.Errorf("accepted %s", s)
		}
	}
}
func FuzzNekoReader(f *testing.F) {
	f.Add([]byte{2, 0, 0, 0, 128})
	f.Fuzz(func(t *testing.T, b []byte) { r := &nekoReader{b: b}; _ = r.str(); _ = r.int(); _ = r.bool() })
}
