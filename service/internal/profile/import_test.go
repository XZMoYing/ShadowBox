package profile

import (
	"encoding/base64"
	"testing"
)

func TestCommonImportFormats(t *testing.T) {
	cases := []struct{ name, text, typ string }{
		{"SS SIP002", "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret")) + "@[::1]:443#test", "shadowsocks"},
		{"VLESS Reality", "vless://00000000-0000-0000-0000-000000000001@example.com:443?security=reality&pbk=public&sid=0123&fp=chrome#reality", "vless"},
		{"Hysteria2", "hysteria2://secret@example.com:443?obfs=salamander&obfs-password=x#hy", "hysteria2"},
		{"Base64 list", base64.StdEncoding.EncodeToString([]byte("trojan://secret@example.com:443#t\nsocks://127.0.0.1:1080#s")), "trojan"},
		{"Clash", "proxies:\n  - name: test\n    type: trojan\n    server: example.com\n    port: 443\n    password: secret\n    sni: example.com\n", "trojan"},
		{"sing-box nodes", "{\"outbounds\":[{\"type\":\"direct\",\"tag\":\"direct\"},{\"type\":\"socks\",\"tag\":\"node\",\"server\":\"localhost\",\"server_port\":1080}]}", "socks"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Parse(c.text)
			if len(r.Errors) > 0 || len(r.Nodes) == 0 {
				t.Fatalf("%+v", r)
			}
			if r.Nodes[0].Type() != c.typ {
				t.Fatal(r.Nodes[0].Type())
			}
		})
	}
}
func TestImportDoesNotSilentlyAcceptUnsupportedLinks(t *testing.T) {
	r := Parse("ssr://bad\nvless://bad")
	if len(r.Errors) != 2 {
		t.Fatalf("expected two errors, got %+v", r)
	}
}
