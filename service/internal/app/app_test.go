package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shadowbox/service/internal/profile"
	"strings"
	"testing"
)

func TestAPIRequiresLocalAuthenticationAndRejectsBrowserOrigin(t *testing.T) {
	a, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	h := a.Handler("secret")
	for _, c := range []struct {
		auth, origin string
		want         int
	}{{"", "", 401}, {"Bearer secret", "https://untrusted.invalid", 403}, {"Bearer secret", "", 200}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1/v1/status", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Authorization", c.auth)
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("got %d want %d", w.Code, c.want)
		}
	}
}
func TestSubscriptionRefreshPreservesIdentityAndExistingDataOnFailure(t *testing.T) {
	a, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	text := "trojan://secret@example.com:443#first"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(text)) }))
	defer server.Close()
	sub := profile.Subscription{ID: "sub", Name: "source", URL: server.URL, IntervalHours: 24}
	if e = a.Refresh(sub); e != nil {
		t.Fatal(e)
	}
	nodes, _ := a.Nodes()
	id := nodes[0].ID
	nodes[0].Favorite = true
	_ = a.Store.Put("nodes", id, nodes[0])
	text = "trojan://secret@example.com:443#renamed"
	if e = a.Refresh(sub); e != nil {
		t.Fatal(e)
	}
	nodes, _ = a.Nodes()
	if len(nodes) != 1 || nodes[0].ID != id || !nodes[0].Favorite || nodes[0].Name != "renamed" {
		t.Fatalf("lost identity %+v", nodes)
	}
	text = "invalid"
	if e = a.Refresh(sub); e == nil {
		t.Fatal("accepted broken subscription")
	}
	nodes, _ = a.Nodes()
	if len(nodes) != 1 || nodes[0].ID != id {
		t.Fatal("failed refresh changed stored nodes")
	}
}
func TestInvalidSettingsAreNotPersisted(t *testing.T) {
	a, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	old := a.Settings()
	h := a.Handler("secret")
	s := old
	s.Port = 80
	raw, _ := json.Marshal(s)
	r := httptest.NewRequest("POST", "http://127.0.0.1/v1/settings", strings.NewReader(string(raw)))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 || a.Settings().Port != old.Port {
		t.Fatalf("invalid port persisted %d", w.Code)
	}
}

func TestImportedPluginCannotInjectCommands(t *testing.T) {
	a, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	n := profile.Node{ID: "injection", Name: "untrusted", External: &profile.External{Kind: "trojan-go", Config: map[string]any{"remote_addr": "example.com", "transport_plugin": map[string]any{"enabled": true, "command": "untrusted-program"}}}}
	if e = a.SaveNodes([]profile.Node{n}, ""); e == nil {
		t.Fatal("command injection was accepted")
	}
	nodes, _ := a.Nodes()
	if len(nodes) != 0 {
		t.Fatal("untrusted plugin was persisted")
	}
}
