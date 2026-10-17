package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const registryPath = `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

func Snapshot() (ProxySnapshot, error) {
	v, e := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$p=Get-ItemProperty '`+registryPath+`'; @{Enable=$p.ProxyEnable;Server=$p.ProxyServer;Override=$p.ProxyOverride;AutoConfigURL=$p.AutoConfigURL}|ConvertTo-Json -Compress`)
	if e != nil {
		return ProxySnapshot{}, e
	}
	return ProxySnapshot{Entries: []ProxyEntry{{Kind: "windows", Raw: v}}}, nil
}
func Apply(port int, s ProxySnapshot) error {
	if len(s.Entries) == 0 {
		return fmt.Errorf("缺少系统代理快照")
	}
	var old map[string]any
	if e := json.Unmarshal([]byte(s.Entries[0].Raw), &old); e != nil {
		return e
	}
	if v, ok := old["AutoConfigURL"].(string); ok && v != "" {
		return fmt.Errorf("当前使用 PAC 自动代理，请先关闭 PAC 或选择 TUN/仅本地代理")
	}

	_, e := run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `Set-ItemProperty '`+registryPath+`' ProxyServer '127.0.0.1:`+strconv.Itoa(port)+`';Set-ItemProperty '`+registryPath+`' ProxyEnable 1;`+notifyScript)
	return e
}
func Restore(s ProxySnapshot, port int) error {
	cur, e := Snapshot()
	if e != nil {
		return e
	}
	var now map[string]any
	_ = json.Unmarshal([]byte(cur.Entries[0].Raw), &now)
	if fmt.Sprint(now["Server"]) != "127.0.0.1:"+strconv.Itoa(port) {
		return nil
	}
	var old map[string]any
	if len(s.Entries) == 0 {
		return nil
	}
	if e = json.Unmarshal([]byte(s.Entries[0].Raw), &old); e != nil {
		return e
	}
	esc := func(v any) string {
		if v == nil {
			return ""
		}
		return strings.ReplaceAll(fmt.Sprint(v), "'", "''")
	}
	_, e = run("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `Set-ItemProperty '`+registryPath+`' ProxyServer '`+esc(old["Server"])+`';Set-ItemProperty '`+registryPath+`' ProxyEnable `+defaultZero(old["Enable"])+`;`+notifyScript)
	return e
}
func defaultZero(v any) string {
	if v == nil {
		return "0"
	}
	if fmt.Sprint(v) == "1" {
		return "1"
	}
	return "0"
}

const notifyScript = `Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public class SBWinInet{[DllImport("wininet.dll",SetLastError=true)]public static extern bool InternetSetOption(IntPtr h,int o,IntPtr b,int l);}';[SBWinInet]::InternetSetOption([IntPtr]::Zero,39,[IntPtr]::Zero,0)|Out-Null;[SBWinInet]::InternetSetOption([IntPtr]::Zero,37,[IntPtr]::Zero,0)|Out-Null`
