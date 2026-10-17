package app

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shadowbox/service/internal/profile"
)

func response(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
}
func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		failure(w, fmt.Errorf("请求格式错误：%w", e))
		return false
	}
	return true
}
func (a *App) Handler(token string) http.Handler {
	mux := http.NewServeMux()
	tests := make(chan struct{}, 4)
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) { response(w, a.Status(true)) })
	mux.HandleFunc("GET /v1/menu", func(w http.ResponseWriter, r *http.Request) {
		nodes, e := a.Nodes()
		if e != nil {
			failure(w, e)
			return
		}
		items := []map[string]any{}
		for _, n := range nodes {
			if n.Favorite {
				items = append(items, map[string]any{"id": n.ID, "name": "★ " + n.Name})
			}
		}
		for _, n := range nodes {
			if !n.Favorite && len(items) < 12 {
				items = append(items, map[string]any{"id": n.ID, "name": n.Name})
			}
		}
		if len(items) > 12 {
			items = items[:12]
		}
		response(w, items)
	})
	mux.HandleFunc("GET /v1/settings", func(w http.ResponseWriter, r *http.Request) { response(w, a.Settings()) })
	mux.HandleFunc("POST /v1/settings", func(w http.ResponseWriter, r *http.Request) {
		s := a.Settings()
		if !readBody(w, r, &s) {
			return
		}
		if e := a.SaveSettings(s); e != nil {
			failure(w, e)
			return
		}
		response(w, s)
	})
	mux.HandleFunc("GET /v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		nodes, e := a.Nodes()
		if e != nil {
			failure(w, e)
			return
		}
		response(w, nodes)
	})
	mux.HandleFunc("POST /v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Nodes []profile.Node `json:"nodes"`
		}
		if !readBody(w, r, &input) {
			return
		}
		if e := a.SaveNodes(input.Nodes, ""); e != nil {
			failure(w, e)
			return
		}
		response(w, map[string]any{"count": len(input.Nodes)})
	})
	mux.HandleFunc("DELETE /v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if e := a.DeleteNode(r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/import/preview", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Text string `json:"text"`
		}
		if !readBody(w, r, &input) {
			return
		}
		response(w, profile.Parse(input.Text))
	})
	mux.HandleFunc("POST /v1/import/file", func(w http.ResponseWriter, r *http.Request) {
		b, e := io.ReadAll(io.LimitReader(r.Body, (16<<20)+1))
		if e != nil || len(b) > 16<<20 {
			failure(w, fmt.Errorf("文件过大或不可读"))
			return
		}
		if strings.HasSuffix(strings.ToLower(r.Header.Get("X-Filename")), ".zip") {
			response(w, profile.ParseZip(b))
		} else {
			response(w, profile.Parse(string(b)))
		}
	})
	mux.HandleFunc("POST /v1/select", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if !readBody(w, r, &in) {
			return
		}
		if e := a.Select(in.ID); e != nil {
			failure(w, e)
			return
		}
		response(w, a.Status(false))
	})
	mux.HandleFunc("POST /v1/connect", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if !readBody(w, r, &in) {
			return
		}
		if e := a.Connect(in.ID); e != nil {
			failure(w, e)
			return
		}
		response(w, a.Status(false))
	})
	mux.HandleFunc("POST /v1/stop", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Stop(); e != nil {
			failure(w, e)
			return
		}
		response(w, a.Status(false))
	})
	mux.HandleFunc("POST /v1/test", func(w http.ResponseWriter, r *http.Request) {
		select {
		case tests <- struct{}{}:
			defer func() { <-tests }()
		default:
			failure(w, fmt.Errorf("已有 4 个测试正在运行"))
			return
		}
		var in struct {
			ID string `json:"id"`
		}
		if !readBody(w, r, &in) {
			return
		}
		delay, e := a.TestNode(in.ID)
		if e != nil {
			failure(w, e)
			return
		}
		response(w, map[string]any{"latency_ms": delay})
	})
	mux.HandleFunc("GET /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		subs := []profile.Subscription{}
		if e := a.Store.All("subscriptions", &subs); e != nil {
			failure(w, e)
			return
		}
		response(w, subs)
	})
	mux.HandleFunc("POST /v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var sub profile.Subscription
		if !readBody(w, r, &sub) {
			return
		}
		if sub.ID == "" {
			sub.ID = profile.NewID()
		}
		if strings.TrimSpace(sub.Name) == "" {
			failure(w, fmt.Errorf("请填写订阅名称"))
			return
		}
		if sub.IntervalHours < 1 {
			sub.IntervalHours = 24
		}
		if e := a.Refresh(sub); e != nil {
			sub.Error = e.Error()
			_ = a.Store.Put("subscriptions", sub.ID, sub)
			failure(w, e)
			return
		}
		a.notify()
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/subscriptions/{id}/refresh", func(w http.ResponseWriter, r *http.Request) {
		var sub profile.Subscription
		if e := a.Store.Get("subscriptions", r.PathValue("id"), &sub); e != nil {
			failure(w, e)
			return
		}
		if e := a.Refresh(sub); e != nil {
			sub.Error = e.Error()
			_ = a.Store.Put("subscriptions", sub.ID, sub)
			failure(w, e)
			return
		}
		a.notify()
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /v1/subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		nodes, e := a.Nodes()
		if e != nil {
			failure(w, e)
			return
		}
		for _, n := range nodes {
			if n.Source == id {
				n.Source = ""
				if e = a.Store.Put("nodes", n.ID, n); e != nil {
					failure(w, e)
					return
				}
			}
		}
		if e = a.Store.Delete("subscriptions", id); e != nil {
			failure(w, e)
			return
		}
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /v1/plugins", func(w http.ResponseWriter, r *http.Request) {
		p := []profile.Plugin{}
		if e := a.Store.All("plugins", &p); e != nil {
			failure(w, e)
			return
		}
		response(w, p)
	})
	mux.HandleFunc("POST /v1/plugins", func(w http.ResponseWriter, r *http.Request) {
		var p profile.Plugin
		if !readBody(w, r, &p) {
			return
		}
		if p.Kind != "naive" && p.Kind != "trojan-go" && p.Kind != "mieru" {
			failure(w, fmt.Errorf("未知插件类型"))
			return
		}
		if !filepath.IsAbs(p.Path) {
			failure(w, fmt.Errorf("请选择插件程序的绝对路径"))
			return
		}
		st, e := os.Stat(p.Path)
		if e != nil || st.IsDir() {
			failure(w, fmt.Errorf("插件文件不可用"))
			return
		}
		if p.ID == "" {
			p.ID = profile.NewID()
		}
		if p.Name == "" {
			p.Name = p.Kind
		}
		if e = a.Store.Put("plugins", p.ID, p); e != nil {
			failure(w, e)
			return
		}
		response(w, p)
	})
	mux.HandleFunc("DELETE /v1/plugins/{id}", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Store.Delete("plugins", r.PathValue("id")); e != nil {
			failure(w, e)
			return
		}
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /v1/logs", func(w http.ResponseWriter, r *http.Request) {
		a.logsMu.Lock()
		entries := append([]LogEntry{}, a.logs...)
		a.logsMu.Unlock()
		response(w, entries)
	})
	mux.HandleFunc("GET /v1/connections", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		if a.instance != nil {
			response(w, a.instance.Stats(true))
		} else {
			response(w, map[string]any{"items": []any{}})
		}
	})
	mux.HandleFunc("POST /v1/ui/show", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.wake++
		a.mu.Unlock()
		a.notify()
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /v1/events", func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		ch := make(chan struct{}, 1)
		a.eventMu.Lock()
		a.listeners[ch] = true
		a.eventMu.Unlock()
		defer func() { a.eventMu.Lock(); delete(a.listeners, ch); a.eventMu.Unlock() }()
		tick := time.NewTicker(25 * time.Second)
		defer tick.Stop()
		send := func() { b, _ := json.Marshal(a.Status(false)); _, _ = fmt.Fprintf(w, "data: %s\n\n", b); f.Flush() }
		send()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				send()
			case <-tick.C:
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
				f.Flush()
			}
		}
	})
	mux.HandleFunc("GET /v1/backup", func(w http.ResponseWriter, r *http.Request) {
		nodes, e := a.Nodes()
		if e != nil {
			failure(w, e)
			return
		}
		subs := []profile.Subscription{}
		_ = a.Store.All("subscriptions", &subs)
		response(w, map[string]any{"format": "shadowbox-backup", "version": 1, "nodes": nodes, "subscriptions": subs, "settings": a.Settings()})
	})
	mux.HandleFunc("POST /v1/backup", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Format        string                 `json:"format"`
			Version       int                    `json:"version"`
			Nodes         []profile.Node         `json:"nodes"`
			Subscriptions []profile.Subscription `json:"subscriptions"`
			Settings      profile.Settings       `json:"settings"`
		}
		if !readBody(w, r, &b) {
			return
		}
		if b.Format != "shadowbox-backup" || b.Version != 1 {
			failure(w, fmt.Errorf("备份格式或版本不支持"))
			return
		}
		if e := b.Settings.Validate(); e != nil {
			failure(w, e)
			return
		}
		if e := a.SaveNodes(b.Nodes, ""); e != nil {
			failure(w, e)
			return
		}
		for _, s := range b.Subscriptions {
			if e := a.Store.Put("subscriptions", s.ID, s); e != nil {
				failure(w, e)
				return
			}
		}
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/quit", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.quitRequested = true
		a.mu.Unlock()
		a.notify()
		response(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if e := a.Stop(); e != nil {
			a.mu.Lock()
			a.quitRequested = false
			a.mu.Unlock()
			a.notify()
			failure(w, e)
			return
		}
		response(w, map[string]bool{"ok": true})
		go func() {
			time.Sleep(200 * time.Millisecond)
			select {
			case <-a.Shutdown:
			default:
				close(a.Shutdown)
			}
		}()
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil || host != "127.0.0.1" || r.Header.Get("Origin") != "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
