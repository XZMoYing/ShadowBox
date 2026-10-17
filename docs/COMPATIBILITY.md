# 0.1.0 兼容范围

基准：NekoBox 官方最新 release 1.4.2（本次开发开始时核对），MatsuriDayo/sing-box 提交 `aed32ee3066cdbc7d471e3e0415c5134088962df`。支持解析不等于已在真实服务端完成所有协议联调。

| 输入或功能 | 当前范围 |
|---|---|
| SS | SIP002 与旧 Base64 链接；内核支持的密码算法；obfs-local/v2ray-plugin 由同款内核适配 |
| VMess / VLESS | VMess JSON Base64；VLESS URI；TCP、WS、HTTP/2、gRPC、HTTP Upgrade、QUIC；TLS/Reality |
| Trojan / AnyTLS | URI、出站 JSON、当前版本 sn 分享；TLS 选项 |
| Hysteria / Hysteria 2 / TUIC | 常见 URI、当前版本 sn 分享、出站 JSON；TUIC v5；HY2 端口跳跃由核心处理 |
| SOCKS / HTTP(S) / SSH | 常见 URI、sn 分享、出站 JSON；SSH 私钥可由高级 JSON 或 sn 导入 |
| WireGuard | 单 peer 的 .conf、当前版本 sn 分享和出站 JSON；多 peer 请手动转换 |
| Naive / Trojan-Go / Mieru | 本机插件路径、私有临时配置、按需启动和停止；插件不随首版捆绑 |
| 代理链 | 第一项入口，最后一项出口；最多 8 跳；外部插件及 QUIC/WireGuard 仅可位于入口 |
| 订阅 | URI 列表、Base64、Clash proxies、sing-box outbounds；不执行订阅中的脚本或下载程序 |
| 文件 | 文本、JSON、YAML、.conf、ZIP；ZIP 最多 100 文件，单文件 16 MiB、总计 32 MiB |
| 二维码 | 从图片解码，不含摄像头或屏幕扫描 |
| 备份 | ShadowBox v1 格式，合并节点和订阅；插件绝对路径不备份，不自动改动网络接管设置 |

`sn://` 是 NekoBox 内部版本格式。当前按 1.4.2 最新字段实现 SS/SOCKS/VMess（含 VLESS）/HTTP/Trojan/AnyTLS/Hysteria/TUIC/WireGuard/SSH/Naive/Trojan-Go/Mieru/自定义出站。旧 bean 版本、NekoBox 完整数据库备份、`neko` 插件 bean、自定义完整配置与全局配置补丁暂不支持，显示错误。Naive 自定义证书/SNI/UoT、Trojan-Go 额外 SIP003 可执行插件、TUIC v4/独立插件 JSON 等也会明确拒绝。

完整 sing-box JSON 目前提取其中的普通出站；不会导入其入站、路由规则、DNS、selector/urltest。引用其他出站 tag 的配置需要在 ShadowBox 中重建代理链。Clash 全量规则和 provider 不在节点订阅导入范围内。不要把这些导入当成完整配置迁移。

手动表单覆盖常见参数，编辑时保留原出站的高级字段；使用“高级 JSON”编辑额外协议选项。ShadowTLS 等同款核心支持但没有专用 URI 解析器的协议可通过出站 JSON 导入。

外部插件配置只允许网络相关字段，不允许通过订阅注入命令、文件路径或额外进程。内核已移除或不支持的参数以其实际校验结果为准。外部插件的版本与服务端兼容性需单独确认。

TUN 初次连接请求系统授权；每次重新启动网络 worker 可能重新弹出授权。当前没有常驻 root helper 的免重复授权服务。Windows Wintun 已由固定版本 sing-tun 嵌入，无需另外加载来历不明的 DLL。

系统代理模式不会覆盖现有的 PAC 自动代理或认证代理。遇到这类配置会提示先手动处理，或选择 TUN/仅本地代理。Windows WPAD 自动发现及 Linux GNOME 以外桌面的系统代理仍需原生验证/适配。
