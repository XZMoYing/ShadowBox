package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"shadowbox/service/internal/profile"
)

func connectProxy(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	count := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			http.Error(w, "CONNECT required", 405)
			return
		}
		remote, e := net.DialTimeout("tcp", r.Host, 3*time.Second)
		if e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		client, buf, e := w.(http.Hijacker).Hijack()
		if e != nil {
			remote.Close()
			return
		}
		count.Add(1)
		defer client.Close()
		defer remote.Close()
		_, _ = fmt.Fprint(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(remote, buf); remote.Close() }()
		_, _ = io.Copy(client, remote)
	}))
	t.Cleanup(server.Close)
	return server, count
}
func proxyNode(server *httptest.Server, id string) profile.Node {
	u, _ := url.Parse(server.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	var p int
	_, _ = fmt.Sscan(port, &p)
	return profile.Node{ID: id, Name: id, Outbound: map[string]any{"type": "http", "server": host, "server_port": p}}
}
func port(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}
func TestRealTrafficThroughSingleNodeAndChain(t *testing.T) {
	if Version != "1.12.19-neko-1" {
		t.Fatalf("unexpected core %s", Version)
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "shadowbox-real-proxy-test") }))
	defer origin.Close()
	for _, hops := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d-hops", hops), func(t *testing.T) {
			a, ac := connectProxy(t)
			chain := []profile.Node{proxyNode(a, "entry")}
			var bc *atomic.Int64
			if hops == 2 {
				b, count := connectProxy(t)
				bc = count
				chain = append(chain, proxyNode(b, "exit"))
			}
			s := profile.Defaults()
			s.Capture = "manual"
			s.Routing = "global"
			s.Port = port(t)
			cfg, e := Compile(chain, s, nil)
			if e != nil {
				t.Fatal(e)
			}
			i, e := Start(cfg, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer i.Close()
			u, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", s.Port))
			tr := &http.Transport{Proxy: http.ProxyURL(u)}
			defer tr.CloseIdleConnections()
			client := http.Client{Transport: tr, Timeout: 8 * time.Second}
			resp, e := client.Get(origin.URL)
			if e != nil {
				t.Fatal(e)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) != "shadowbox-real-proxy-test" {
				t.Fatal(string(body))
			}
			if ac.Load() != 1 {
				t.Fatalf("entry proxy was bypassed (%d)", ac.Load())
			}
			if bc != nil && bc.Load() != 1 {
				t.Fatalf("exit proxy was bypassed (%d)", bc.Load())
			}
			stats := i.Stats(false)
			if stats["download"].(int64) == 0 {
				t.Fatal("traffic counters did not record the request")
			}
		})
	}
}
func TestChainValidationAndPrivilegedConfig(t *testing.T) {
	a := profile.Node{ID: "a", Chain: []string{"b"}}
	b := profile.Node{ID: "b", Chain: []string{"a"}}
	if _, e := Resolve("a", []profile.Node{a, b}); e == nil {
		t.Fatal("accepted a cycle")
	}
	if _, e := Resolve("missing", nil); e == nil {
		t.Fatal("accepted a missing reference")
	}
	if e := ValidatePrivileged([]byte(`{"outbounds":[{"tls":{"certificate_path":"/etc/private"}}]}`)); e == nil {
		t.Fatal("accepted an elevated local-file reference")
	}
	s := profile.Defaults()
	s.Capture = "tun"
	cfg, e := Compile([]profile.Node{{ID: "a", Outbound: map[string]any{"type": "socks", "server": "127.0.0.1", "server_port": 1080}}}, s, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidatePrivileged(cfg); e != nil {
		t.Fatal(e)
	}
	if _, _, e = Options(cfg); e != nil {
		t.Fatal(e)
	}
}

func TestNekoFixturesDecodeAsCoreOptions(t *testing.T) {
	b, e := os.ReadFile("../profile/testdata/neko-1.4.2.txt")
	if e != nil {
		t.Fatal(e)
	}
	r := profile.Parse(string(b))
	if len(r.Errors) > 0 {
		t.Fatal(r.Errors)
	}
	for _, n := range r.Nodes {
		if n.External != nil {
			continue
		}
		cfg, e := Compile([]profile.Node{n}, profile.Defaults(), nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = Options(cfg); e != nil {
			t.Errorf("%s: %v", n.Type(), e)
		}
	}
}
