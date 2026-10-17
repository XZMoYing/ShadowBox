package engine

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"shadowbox/service/internal/profile"
	"testing"
	"time"
)

// Real client/server roundtrips exercise URI decoding, configuration generation,
// authentication and transport using the pinned engine on both ends.
func TestImportedProtocolsCarryRealTraffic(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "authenticated-core-roundtrip") }))
	defer origin.Close()
	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer certServer.Close()
	cert := certServer.TLS.Certificates[0]
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}))
	key, e := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if e != nil {
		t.Fatal(e)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	uuid := "22222222-2222-4222-8222-222222222222"
	for _, protocol := range []string{"shadowsocks", "vless-ws", "trojan-tls"} {
		t.Run(protocol, func(t *testing.T) {
			listen := port(t)
			in := map[string]any{"tag": "in", "listen": "127.0.0.1", "listen_port": listen}
			var uri string
			switch protocol {
			case "shadowsocks":
				in["type"] = "shadowsocks"
				in["method"] = "aes-128-gcm"
				in["password"] = "roundtrip-password"
				uri = fmt.Sprintf("ss://%s@127.0.0.1:%d#fixture", base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:roundtrip-password")), listen)
			case "vless-ws":
				in["type"] = "vless"
				in["users"] = []any{map[string]any{"uuid": uuid}}
				in["transport"] = map[string]any{"type": "ws", "path": "/socket"}
				uri = fmt.Sprintf("vless://%s@127.0.0.1:%d?type=ws&path=%%2Fsocket#fixture", uuid, listen)
			case "trojan-tls":
				in["type"] = "trojan"
				in["users"] = []any{map[string]any{"password": "roundtrip-password"}}
				in["tls"] = map[string]any{"enabled": true, "certificate": []string{certPEM}, "key": []string{keyPEM}}
				uri = fmt.Sprintf("trojan://roundtrip-password@127.0.0.1:%d#fixture", listen)
			}
			data, _ := json.Marshal(map[string]any{"inbounds": []any{in}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}, "experimental": map[string]any{"clash_api": map[string]any{}}})
			server, e := Start(data, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer server.Close()
			imported := profile.Parse(uri)
			if len(imported.Errors) > 0 || len(imported.Nodes) != 1 {
				t.Fatal(imported.Errors)
			}
			n := imported.Nodes[0]
			if protocol == "trojan-tls" {
				n.Outbound["tls"].(map[string]any)["certificate"] = []string{certPEM}
			}
			settings := profile.Defaults()
			settings.Capture = "manual"
			settings.Routing = "global"
			settings.Port = port(t)
			cfg, e := Compile([]profile.Node{n}, settings, nil)
			if e != nil {
				t.Fatal(e)
			}
			clientCore, e := Start(cfg, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer clientCore.Close()
			proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", settings.Port))
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			defer transport.CloseIdleConnections()
			client := http.Client{Transport: transport, Timeout: 5 * time.Second}
			resp, e := client.Get(origin.URL)
			if e != nil {
				t.Fatal(e)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if string(body) != "authenticated-core-roundtrip" {
				t.Fatal(string(body))
			}
			if server.Stats(false)["download"].(int64) == 0 {
				t.Fatal("proxy server was bypassed")
			}
		})
	}
}
