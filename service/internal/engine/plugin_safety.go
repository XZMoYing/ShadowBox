package engine

import (
	"fmt"
	"shadowbox/service/internal/profile"
)

// A subscription is data, never permission to spawn a command or write a file.
// Keep the plugin inputs to the network fields supported by the desktop adapter.
func ValidateExternal(e *profile.External) error {
	if e == nil {
		return nil
	}
	allowed := map[string]bool{}
	keys := []string{}
	switch e.Kind {
	case "naive":
		keys = []string{"proxy", "extra-headers", "insecure-concurrency"}
	case "trojan-go":
		keys = []string{"remote_addr", "remote_port", "password", "ssl", "websocket", "shadowsocks", "tcp"}
	case "mieru":
		keys = []string{"activeProfile", "loggingLevel", "profiles"}
	default:
		return fmt.Errorf("不支持的插件类型 %s", e.Kind)
	}
	for _, k := range keys {
		allowed[k] = true
	}
	for k := range e.Config {
		if !allowed[k] {
			return fmt.Errorf("%s 插件配置不允许字段 %s", e.Kind, k)
		}
	}
	var walk func(any) error
	walk = func(v any) error {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				switch k {
				case "command", "args", "env", "plugin", "transport_plugin", "cert", "key", "cert_path", "key_path", "log_file", "logFile", "logPath", "configFile", "socks5Authentication":
					return fmt.Errorf("插件配置不允许命令、文件或额外认证字段 %s", k)
				}
				if e := walk(v); e != nil {
					return e
				}
			}
		case []any:
			for _, v := range x {
				if e := walk(v); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return walk(e.Config)
}
