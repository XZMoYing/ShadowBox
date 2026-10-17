package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	S "github.com/sagernet/sing/common/metadata"
	"shadowbox/service/internal/engine"
	"shadowbox/service/internal/platform"
	"shadowbox/service/internal/profile"
	"shadowbox/service/internal/store"
)

type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
type App struct {
	Store          *store.Store
	Dir            string
	Shutdown       chan struct{}
	op             sync.Mutex
	mu             sync.RWMutex
	logsMu         sync.Mutex
	settings       profile.Settings
	selected       string
	activeID       string
	activeName     string
	busy           bool
	lastError      string
	started        time.Time
	wake           uint64
	quitRequested  bool
	revision       uint64
	instance       *engine.Instance
	tun            *engine.TunProcess
	plugins        []*engine.PluginProcess
	snapshot       *platform.ProxySnapshot
	activeSettings profile.Settings
	healthCancel   chan struct{}
	generation     uint64
	logs           []LogEntry
	eventMu        sync.Mutex
	listeners      map[chan struct{}]bool
}

func New(dir string) (*App, error) {
	engine.RecoverTun(dir)
	s, e := store.Open(dir)
	if e != nil {
		return nil, e
	}
	a := &App{Store: s, Dir: dir, Shutdown: make(chan struct{}), settings: profile.Defaults(), listeners: map[chan struct{}]bool{}}
	_ = s.Get("state", "settings", &a.settings)
	_ = s.Get("state", "selected", &a.selected)
	if snap, e := platform.LoadSnapshot(dir); e == nil {
		var p int
		_ = s.Get("state", "recovery_port", &p)
		if p == 0 {
			p = a.settings.Port
		}
		if e = platform.Restore(snap, p); e != nil {
			a.Log("error", "上次退出后的系统代理恢复失败："+e.Error())
		} else {
			_ = platform.ClearSnapshot(dir)
		}
	}
	return a, nil
}
func (a *App) Log(level, message string) {
	a.logsMu.Lock()
	defer a.logsMu.Unlock()
	if len(message) > 2048 {
		message = message[:2048]
	}
	a.logs = append(a.logs, LogEntry{time.Now(), level, message})
	if len(a.logs) > 300 {
		a.logs = append([]LogEntry(nil), a.logs[len(a.logs)-300:]...)
	}
}
func (a *App) notify() {
	a.mu.Lock()
	a.revision++
	a.mu.Unlock()
	a.eventMu.Lock()
	defer a.eventMu.Unlock()
	for c := range a.listeners {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}
func (a *App) Status(detail bool) map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	stats := map[string]any{"upload": 0, "download": 0, "connections": 0}
	if detail {
		if a.instance != nil {
			stats = a.instance.Stats(false)
		} else if a.tun != nil {
			stats = a.tun.Stats()
		}
	}
	return map[string]any{"quit_requested": a.quitRequested, "version": "0.1.0", "revision": a.revision, "core_version": engine.Version, "running": a.activeID != "", "busy": a.busy, "selected": a.selected, "active_id": a.activeID, "active_name": a.activeName, "capture": a.settings.Capture, "routing": a.settings.Routing, "port": a.settings.Port, "error": a.lastError, "started_at": a.started, "wake_generation": a.wake, "stats": stats, "platform": runtime.GOOS}
}
func (a *App) Nodes() ([]profile.Node, error) {
	nodes := []profile.Node{}
	e := a.Store.All("nodes", &nodes)
	return nodes, e
}
func (a *App) Settings() profile.Settings { a.mu.RLock(); defer a.mu.RUnlock(); return a.settings }
func (a *App) setBusy(b bool, err error) {
	a.mu.Lock()
	a.busy = b
	if err != nil {
		a.lastError = err.Error()
		a.Log("error", err.Error())
	} else if b {
		a.lastError = ""
	}
	a.mu.Unlock()
	a.notify()
}
func (a *App) Connect(id string) (err error) {
	a.op.Lock()
	defer a.op.Unlock()
	a.setBusy(true, nil)
	defer func() { a.setBusy(false, err) }()
	if id == "" {
		a.mu.RLock()
		id = a.selected
		a.mu.RUnlock()
	}
	return a.connect(id)
}
func (a *App) connect(id string) error {
	nodes, e := a.Nodes()
	if e != nil {
		return e
	}
	chain, e := engine.Resolve(id, nodes)
	if e != nil {
		return e
	}
	var name string
	for _, n := range nodes {
		if n.ID == id {
			name = n.Name
		}
	}
	s := a.Settings()
	var plugins []profile.Plugin
	if e = a.Store.All("plugins", &plugins); e != nil {
		return e
	}
	if e = a.stop(); e != nil {
		return e
	}
	ports, processes, e := engine.StartPlugins(chain, plugins, a.Dir)
	if e != nil {
		return e
	}
	success := false
	defer func() {
		if !success {
			engine.StopPlugins(processes)
		}
	}()
	cfg, e := engine.Compile(chain, s, ports)
	if e != nil {
		return e
	}
	var snap *platform.ProxySnapshot
	var inst *engine.Instance
	var tun *engine.TunProcess
	if s.Capture == "system" {
		v, e := platform.Snapshot()
		if e != nil {
			return e
		}
		snap = &v
	}
	if s.Capture == "tun" {
		cfg, e = engine.ProtectPluginRoutes(cfg, chain)
		if e == nil {
			tun, e = engine.StartTun(cfg, a.Dir)
		}
	} else {
		inst, e = engine.Start(cfg, a.Log)
	}
	if e != nil {
		return e
	}
	if snap != nil {
		if e = a.Store.Put("state", "recovery_port", s.Port); e == nil {
			e = platform.SaveSnapshot(a.Dir, *snap)
		}
		if e == nil {
			e = platform.Apply(s.Port, *snap)
		}
		if e != nil {
			restoreErr := platform.Restore(*snap, s.Port)
			_ = inst.Close()
			if restoreErr == nil {
				_ = platform.ClearSnapshot(a.Dir)
			} else {
				return fmt.Errorf("%v；恢复失败：%w", e, restoreErr)
			}
			return e
		}
	}
	a.mu.Lock()
	a.instance = inst
	a.tun = tun
	a.plugins = processes
	a.snapshot = snap
	a.activeID = id
	a.activeName = name
	a.selected = id
	a.started = time.Now()
	a.activeSettings = s
	a.generation++
	generation := a.generation
	healthCancel := make(chan struct{})
	a.healthCancel = healthCancel
	a.lastError = ""
	a.mu.Unlock()
	_ = a.Store.Put("state", "selected", id)
	success = true
	watch := func(done <-chan struct{}, message string) {
		go func() {
			select {
			case <-healthCancel:
				return
			case <-done:
			}
			a.op.Lock()
			defer a.op.Unlock()
			a.mu.RLock()
			current := a.generation == generation && a.activeID != ""
			a.mu.RUnlock()
			if current {
				_ = a.stop()
				a.setBusy(false, fmt.Errorf("%s，请检查配置后重新连接", message))
			}
		}()
	}
	if tun != nil {
		watch(tun.Done(), "TUN 进程已退出")
	}
	for _, p := range processes {
		watch(p.Done(), "外部插件已退出")
	}
	a.Log("info", "已连接 "+name)
	a.notify()
	return nil
}
func (a *App) Stop() (err error) {
	a.op.Lock()
	defer a.op.Unlock()
	a.setBusy(true, nil)
	defer func() { a.setBusy(false, err) }()
	return a.stop()
}
func (a *App) stop() error {
	a.mu.Lock()
	inst, tun, plugins, snapshot, s := a.instance, a.tun, a.plugins, a.snapshot, a.activeSettings
	if a.healthCancel != nil {
		close(a.healthCancel)
		a.healthCancel = nil
	}
	a.generation++
	a.instance = nil
	a.tun = nil
	a.plugins = nil
	a.snapshot = nil
	a.activeID = ""
	a.activeName = ""
	a.mu.Unlock()
	var errs []error
	if snapshot == nil {
		if v, e := platform.LoadSnapshot(a.Dir); e == nil {
			snapshot = &v
			_ = a.Store.Get("state", "recovery_port", &s.Port)
		}
	}
	if snapshot != nil {
		if e := platform.Restore(*snapshot, s.Port); e != nil {
			errs = append(errs, e)
		} else {
			_ = platform.ClearSnapshot(a.Dir)
		}
	}
	if inst != nil {
		if e := inst.Close(); e != nil {
			errs = append(errs, e)
		}
	}
	if tun != nil {
		tun.Close()
	}
	engine.StopPlugins(plugins)
	a.notify()
	return errors.Join(errs...)
}
func (a *App) Select(id string) (err error) {
	a.op.Lock()
	defer a.op.Unlock()
	nodes, e := a.Nodes()
	if e != nil {
		return e
	}
	if _, e = engine.Resolve(id, nodes); e != nil {
		return e
	}
	a.mu.RLock()
	running := a.activeID != ""
	a.mu.RUnlock()
	if running {
		a.setBusy(true, nil)
		defer func() { a.setBusy(false, err) }()
		return a.connect(id)
	}
	if e = a.Store.Put("state", "selected", id); e != nil {
		return e
	}
	a.mu.Lock()
	a.selected = id
	a.mu.Unlock()
	a.notify()
	return nil
}
func (a *App) SaveSettings(s profile.Settings) error {
	if e := s.Validate(); e != nil {
		return e
	}
	a.op.Lock()
	defer a.op.Unlock()
	old := a.Settings()
	a.mu.RLock()
	active := a.activeID
	a.mu.RUnlock()
	b1, _ := json.Marshal(networkSettings(old))
	b2, _ := json.Marshal(networkSettings(s))
	restart := active != "" && string(b1) != string(b2)
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	if restart {
		a.setBusy(true, nil)
		if e := a.connect(active); e != nil {
			a.mu.Lock()
			a.settings = old
			a.mu.Unlock()
			rollback := a.connect(active)
			a.setBusy(false, e)
			if rollback != nil {
				return fmt.Errorf("更改失败：%v；旧连接恢复失败：%w", e, rollback)
			}
			return e
		}
		a.setBusy(false, nil)
	}
	if e := a.Store.Put("state", "settings", s); e != nil {
		return e
	}
	a.notify()
	return nil
}
func networkSettings(s profile.Settings) any {
	return []any{s.Capture, s.Routing, s.Port, s.DNS, s.IPv6, s.DirectDomains, s.ProxyDomains}
}
func (a *App) SaveNodes(nodes []profile.Node, source string) error {
	a.op.Lock()
	defer a.op.Unlock()
	if len(nodes) > 10000 {
		return fmt.Errorf("单次最多导入 10000 个节点")
	}
	for i := range nodes {
		n := &nodes[i]
		if n.ID == "" {
			n.ID = profile.NewID()
		}
		if len(n.ID) > 80 || len(n.Name) > 512 {
			return fmt.Errorf("节点标识或名称过长")
		}
		if n.Group == "" {
			n.Group = "本地节点"
		}
		if e := engine.ValidateExternal(n.External); e != nil {
			return e
		}
		n.UpdatedAt = time.Now()
		if n.Outbound != nil {
			delete(n.Outbound, "tag")
		}
		if len(n.Chain) == 0 && n.External == nil {
			cfg, e := engine.Compile([]profile.Node{*n}, profile.Defaults(), nil)
			if e != nil {
				return fmt.Errorf("%s: %w", n.Name, e)
			}
			if e = engine.Validate(cfg); e != nil {
				return fmt.Errorf("%s: %w", n.Name, e)
			}
		}
	}
	all, e := a.Nodes()
	if e != nil {
		return e
	}
	by := map[string]profile.Node{}
	for _, n := range all {
		by[n.ID] = n
	}
	for _, n := range nodes {
		by[n.ID] = n
	}
	merged := []profile.Node{}
	for _, n := range by {
		merged = append(merged, n)
	}
	for _, n := range nodes {
		if len(n.Chain) > 0 {
			if _, e = engine.Resolve(n.ID, merged); e != nil {
				return e
			}
		}
	}
	if e = a.Store.ReplaceNodes(source, nodes); e != nil {
		return e
	}
	a.mu.Lock()
	if a.selected == "" && len(nodes) > 0 {
		a.selected = nodes[0].ID
		_ = a.Store.Put("state", "selected", a.selected)
	}
	a.mu.Unlock()
	a.notify()
	return nil
}
func (a *App) DeleteNode(id string) error {
	a.op.Lock()
	defer a.op.Unlock()
	a.mu.RLock()
	active := a.activeID
	a.mu.RUnlock()
	nodes, e := a.Nodes()
	if e != nil {
		return e
	}
	if id == active {
		return fmt.Errorf("请先停止或切换正在使用的节点")
	}
	for _, n := range nodes {
		for _, x := range n.Chain {
			if x == id {
				return fmt.Errorf("节点仍被代理链 %s 引用", n.Name)
			}
		}
	}
	if e = a.Store.Delete("nodes", id); e != nil {
		return e
	}
	a.mu.Lock()
	if a.selected == id {
		a.selected = ""
		_ = a.Store.Put("state", "selected", "")
	}
	a.mu.Unlock()
	a.notify()
	return nil
}
func identity(n profile.Node) string {
	m := profile.CloneMap(n.Outbound)
	if m == nil {
		m = map[string]any{}
	}
	delete(m, "tag")
	b, _ := json.Marshal([]any{m, n.External})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (a *App) Refresh(sub profile.Subscription) error {
	u, e := url.Parse(sub.URL)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("订阅必须是有效的 HTTPS 或 HTTP 地址")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", sub.URL, nil)
	req.Header.Set("User-Agent", "NekoBox/1.4.2 ShadowBox/0.1.0")
	client := http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 45 * time.Second}
	defer client.CloseIdleConnections()
	resp, e := client.Do(req)
	if e != nil {
		return fmt.Errorf("订阅下载失败：%v", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("订阅服务器返回 %d", resp.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if e != nil {
		return e
	}
	if len(data) > 16<<20 {
		return fmt.Errorf("订阅超过 16 MiB")
	}
	result := profile.Parse(string(data))
	if len(result.Nodes) == 0 {
		return fmt.Errorf("订阅没有可用节点：%s", strings.Join(result.Errors, "；"))
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("订阅存在 %d 条无法解析的内容，已保留旧节点：%s", len(result.Errors), result.Errors[0])
	}
	old, e := a.Nodes()
	if e != nil {
		return e
	}
	existing := map[string]profile.Node{}
	for _, n := range old {
		if n.Source == sub.ID {
			existing[identity(n)] = n
		}
	}
	newIDs := map[string]bool{}
	for i := range result.Nodes {
		n := &result.Nodes[i]
		if prev, ok := existing[identity(*n)]; ok {
			n.ID = prev.ID
			n.Favorite = prev.Favorite
			n.Latency = prev.Latency
		}
		n.Group = sub.Name
		n.Source = sub.ID
		newIDs[n.ID] = true
	}
	for _, n := range old {
		if n.Source != sub.ID || newIDs[n.ID] {
			continue
		}
		keep := false
		a.mu.RLock()
		keep = a.activeID == n.ID
		a.mu.RUnlock()
		for _, c := range old {
			for _, id := range c.Chain {
				if id == n.ID {
					keep = true
				}
			}
		}
		if keep {
			n.Source = ""
			n.Group = "订阅已移除"
			result.Nodes = append(result.Nodes, n)
		}
	}
	if e = a.SaveNodes(result.Nodes, sub.ID); e != nil {
		return e
	}
	sub.UpdatedAt = time.Now()
	sub.Error = ""
	sub.Info = resp.Header.Get("Subscription-Userinfo")
	if sub.IntervalHours < 1 {
		sub.IntervalHours = 24
	}
	return a.Store.Put("subscriptions", sub.ID, sub)
}
func (a *App) Schedule(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var subs []profile.Subscription
			if a.Store.All("subscriptions", &subs) != nil {
				continue
			}
			for _, s := range subs {
				if s.IntervalHours < 1 || time.Since(s.UpdatedAt) < time.Duration(s.IntervalHours)*time.Hour {
					continue
				}
				if e := a.Refresh(s); e != nil {
					s.Error = e.Error()
					s.UpdatedAt = time.Now()
					_ = a.Store.Put("subscriptions", s.ID, s)
					a.Log("warn", "订阅 "+s.Name+" 更新失败")
				}
			}
		}
	}
}
func (a *App) TestNode(id string) (int64, error) {
	nodes, e := a.Nodes()
	if e != nil {
		return 0, e
	}
	chain, e := engine.Resolve(id, nodes)
	if e != nil {
		return 0, e
	}
	var plugins []profile.Plugin
	_ = a.Store.All("plugins", &plugins)
	ports, ps, e := engine.StartPlugins(chain, plugins, a.Dir)
	if e != nil {
		return 0, e
	}
	defer engine.StopPlugins(ps)
	s := a.Settings()
	s.Capture = "manual"
	s.Routing = "global"
	cfg, e := engine.Compile(chain, s, ports)
	if e != nil {
		return 0, e
	}
	var m map[string]any
	_ = json.Unmarshal(cfg, &m)
	delete(m, "inbounds")
	cfg, _ = json.Marshal(m)
	inst, e := engine.Start(cfg, nil)
	if e != nil {
		return 0, e
	}
	defer inst.Close()
	out, e := inst.Dialer("proxy")
	if e != nil {
		return 0, e
	}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return out.DialContext(ctx, network, S.ParseSocksaddr(address))
	}}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 10 * time.Second}
	start := time.Now()
	resp, e := client.Get(s.TestURL)
	if e != nil {
		return 0, fmt.Errorf("连接测试失败")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("测试地址返回 %d", resp.StatusCode)
	}
	delay := time.Since(start).Milliseconds()
	for _, n := range nodes {
		if n.ID == id {
			n.Latency = delay
			_ = a.Store.Put("nodes", n.ID, n)
		}
	}
	return delay, nil
}
func (a *App) Close() {
	_ = a.Stop()
	_ = a.Store.DB.Close()
	_ = os.Remove(filepath.Join(a.Dir, "service.json"))
}
