package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"time"
)

type Node struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Group     string         `json:"group"`
	Outbound  map[string]any `json:"outbound,omitempty"`
	Chain     []string       `json:"chain,omitempty"`
	External  *External      `json:"external,omitempty"`
	Favorite  bool           `json:"favorite"`
	Latency   int64          `json:"latency_ms"`
	Source    string         `json:"source,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}
type External struct {
	Kind   string         `json:"kind"`
	Config map[string]any `json:"config"`
}
type Subscription struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	IntervalHours int       `json:"interval_hours"`
	UpdatedAt     time.Time `json:"updated_at"`
	Error         string    `json:"error,omitempty"`
	Info          string    `json:"info,omitempty"`
}
type Plugin struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}
type Settings struct {
	Capture       string   `json:"capture"`
	Routing       string   `json:"routing"`
	Port          int      `json:"port"`
	DNS           string   `json:"dns"`
	IPv6          bool     `json:"ipv6"`
	Theme         string   `json:"theme"`
	Color         string   `json:"color"`
	Compact       bool     `json:"compact"`
	DirectDomains []string `json:"direct_domains"`
	ProxyDomains  []string `json:"proxy_domains"`
	AutoConnect   bool     `json:"auto_connect"`
	TestURL       string   `json:"test_url"`
}

func Defaults() Settings {
	return Settings{IPv6: true, Capture: "system", Routing: "rule", Port: 10808, DNS: "1.1.1.1", Theme: "system", Color: "green", TestURL: "https://www.gstatic.com/generate_204"}
}
func (s Settings) Validate() error {
	if s.Port < 1024 || s.Port > 65535 {
		return fmt.Errorf("本地端口必须在 1024–65535 之间")
	}
	if s.Capture != "system" && s.Capture != "manual" && s.Capture != "tun" {
		return fmt.Errorf("未知接管方式")
	}
	if s.Routing != "rule" && s.Routing != "global" && s.Routing != "direct" {
		return fmt.Errorf("未知分流策略")
	}
	if net.ParseIP(s.DNS) == nil {
		return fmt.Errorf("DNS 服务器必须为 IP 地址")
	}
	u, e := url.Parse(s.TestURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("测试地址必须是有效的 HTTP/HTTPS URL")
	}
	if s.Theme != "system" && s.Theme != "light" && s.Theme != "dark" {
		return fmt.Errorf("未知主题")
	}
	switch s.Color {
	case "green", "amber", "coral", "blue", "violet":
	default:
		return fmt.Errorf("未知主题色")
	}
	return nil
}
func NewID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func CloneMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
func (n Node) Type() string {
	if len(n.Chain) > 0 {
		return "chain"
	}
	if n.External != nil {
		return n.External.Kind
	}
	if t, ok := n.Outbound["type"].(string); ok {
		return t
	}
	return "unknown"
}
