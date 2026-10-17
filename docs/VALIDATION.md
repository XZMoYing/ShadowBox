# 0.1.0 验证记录

日期：2026-10-17。主机：macOS 14.7.4 / Apple Silicon / Xcode 16.2。

| 检查 | 结果与范围 |
|---|---|
| macOS release 构建 | Flutter、Rust、Go 和应用封装均通过；本地临时签名，未公证 |
| Go 测试 | 全部通过，开启 race detector 和与 NekoBox 一致的六个构建标签 |
| 实际转发 | 本机 HTTP 代理单跳、双跳；检查每一跳确实被使用及流量计数 |
| 实际协议 | 从分享链接解析配置，使用同款内核作本地客户端和服务端，Shadowsocks AEAD、VLESS WebSocket、Trojan TLS（验证证书）转发通过 |
| 导入与存储 | 常见 URI、Base64、Clash、sing-box、sn 分享样本；订阅改名保留 ID/收藏，失败保留旧节点 |
| sn 样本 | 由 NekoBox 的 Kryo 5.2.1 库生成，覆盖十个样本、中文/emoji、认证、Reality/WS、插件参数 |
| 输入稳健性 | Kryo reader 约 59 万次随机输入模糊测试，无崩溃 |
| 控制接口 | 缺少 token 被拒绝；浏览器 Origin 被拒绝；非法设置和插件命令注入不能保存 |
| Flutter | `dart analyze lib test` 无问题；四项 widget 测试通过：最小窗口下全部页面、手动表单、连接、导入预览 |
| Rust | 原生 Clippy 严格检查通过；Windows GNU 目标 `cargo check` 通过 |
| Windows/Linux Go | 交叉编译通过。该编译检查使用 CGO=0，不作为发行产物；实际发行必须由原生流水线启用 CGO 的 SQLite 构建 |
| 打包冒烟 | 独立测试目录；只使用本地代理，不改变系统网络设置；真实转发、UI 进程结束后转发、重开 UI 不重启核心、后台被强制终止后的托盘恢复通过 |
| 系统代理实机 | 未执行修改网络设置的验收；不能宣称三平台实际恢复行为已经验证 |
| TUN 实机 | 未执行管理员授权后的路由/DNS、睡眠唤醒、异常退出、IPv6 泄漏验收 |
| 实际窗口 | 解锁后通过原生窗口操作验证：连接、节点延迟测试、分享链接预览与导入、文件选择器、浅色/深色、窗口关闭和 Command-Q |
| 关闭与重开 | 实际点击关闭后 Flutter 进程不存在，原服务 PID 和连接开始时间不变，真实转发继续；经本地控制接口重开产生新的 Flutter PID，原连接继续 |
| 原生托盘交互 | 托盘事件/API 生命周期已验证；当前 CUA 无法定位无窗口托盘进程，尚未完成实际菜单点击验收 |
| Windows/Linux 完整 GUI | 原生构建/打包工作流已提供，本次没有对应系统主机和流水线运行结果 |

第一轮打包测量：主界面打开时总 RSS 约 164.5 MiB；独立 UI 进程结束后、单个本地 HTTP 节点连接状态下，十秒采样总 RSS 最大约 54.7 MiB。包括托盘和后台服务。这不是大规则集、TUN、插件、吞吐压力或所有系统的资源保证。最终打包冒烟的机器可读结果保存在 `dist/smoke-macos.json`。

采用独立 macOS 辅助应用后的重复测量：前台采样约 167.6 MiB；关闭界面进程后的十秒后台采样最大约 54.4 MiB。同一轮真实转发、服务崩溃恢复和退出检查全部通过。

macOS 的 `正常关闭窗口 → UI 退出` 已有真实交互证据，记录在 `dist/gui-validation.json`。Command-Q 完整退出后确认托盘、后台和界面进程全部结束。Windows 关闭主窗口退出消息循环、Linux GTK 主窗口生命周期仍需在各自系统实测。

macOS 打包将界面放入独立的 `ShadowBox UI.app` 辅助应用，使用 `dev.shadowbox.ui` 标识；外层 `dev.shadowbox.desktop` 由原生托盘启动。避免托盘和界面共用应用标识导致的窗口定位混淆。

系统代理/TUN 验收脚本为 `scripts/validate_network_macos.py`。`--prepare` 只编译本机 HTTP 客户端并检查路由；实际修改必须显式指定 `--mode system|tun --apply`。测试通过私有数据目录和本机服务完成，检查 CFNetwork 实际转发、TUN 路由及 DNS 劫持，并在 finally 中停止服务、校验原代理和路由恢复。TUN 检测到其他 VPN 路由时拒绝启动，不停止其他代理软件。管理员授权由用户手动完成。

## 正式版前仍需完成

- 在本机补充托盘菜单实际点击、系统主题自动切换及睡眠唤醒验收。
- 使用隔离网络或获得用户同意后验证系统代理接管/恢复，以及管理员授权的 TUN、DNS、默认路由和故障恢复。
- 在 Windows 和 GNOME Linux 运行原生流水线与同样的网络验收，补充其他 Linux 桌面的适配策略。
- 与真实协议服务端和可分发的外部插件版本完成矩阵联调；当前没有捆绑插件可执行程序。
- 配置发布签名及 macOS 公证。公开源码位于 [XZMoYing/ShadowBox](https://github.com/XZMoYing/ShadowBox)，本地构建仍使用临时签名。
