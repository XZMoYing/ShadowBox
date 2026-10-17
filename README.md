# ShadowBox

Windows、macOS、Linux 桌面代理客户端。Flutter Material 3 界面 + Rust 原生托盘 + Go 后台服务，直接编译 NekoBox 1.4.2 使用的 MatsuriDayo sing-box `1.12.19-neko-1`。

**当前版本：0.1.0 桌面预览版。** 本机产物是 Apple Silicon macOS 应用和 DMG。Windows、Linux 有原生适配和打包流水线，需要在对应系统运行流水线及网络验收后发布；本仓库不把跨平台源码当作已完成实机验收。

## 使用

macOS：打开 DMG，将 `ShadowBox.app` 拖入“应用程序”，启动应用。当前本地产物使用临时签名，尚未做 Apple Developer ID 签名和公证；系统可能要求通过系统设置确认打开。不要关闭系统安全检查。

1. 在“节点”中粘贴分享链接、导入文件或二维码图片，也可手动选择协议填写参数。在“订阅”中添加 HTTP/HTTPS 订阅地址。
2. 选择一个节点。接管方式可以选系统代理、TUN 或仅本地代理；分流策略独立选择按规则、全局代理或直连。
3. 点击连接。默认本地 HTTP/SOCKS 混合端口为 `127.0.0.1:10808`，仅本机可访问。
4. **关闭窗口会退出界面进程，托盘和代理服务继续工作。** 从托盘“打开 ShadowBox”，或再次启动应用，可恢复界面。
5. 完全退出请使用托盘“退出并恢复网络”。暂停或正常退出会停止内核、外部插件并恢复本软件接管的系统代理。

默认主题为绿色与暖中性色，不自动使用系统蓝紫配色。自定义主题仍提供蓝色、紫色。界面左上角不放大标题、副标题或宣传 slogan。

## 功能

- 常见节点分享链接、Base64 节点列表、Clash YAML 的 `proxies`、sing-box 出站 JSON、WireGuard 配置、ZIP 文件、二维码图片。
- NekoBox 1.4.2 当前 bean 版本的 `sn://` 分享格式；已用其原版 Kryo 5.2.1 生成的样本校验中文、emoji、认证和传输参数。
- 手动添加和编辑常用协议；高级 JSON 编辑；分组、搜索、收藏、延迟测试、节点导出、ShadowBox 备份导入/导出。
- 订阅定时更新，解析或校验失败时保留旧节点；更新后尽可能保留节点 ID、收藏和代理链引用。
- 最多 8 跳的代理链，检查循环和丢失的引用。列表按实际流量方向排列：第一项为入口，最后一项为出口。
- 外部 Naive、Trojan-Go、Mieru 桌面可执行程序适配；需要自行安装匹配系统和架构的插件，并在设置中选择路径。Android 插件 APK 不能直接使用。
- 系统代理：macOS 网络服务、Windows 当前用户 Internet Settings、Linux GNOME GSettings。其他 Linux 桌面可用本地代理/TUN；GNOME Shell 可能需要 AppIndicator 扩展显示托盘。
- TUN：按需请求管理员授权，独立网络 worker；主界面和托盘始终以普通用户运行。worker 检测到服务心跳丢失后退出。
- 界面进程不存在时不进行界面刷新、连接列表采集或图表更新。托盘以事件流接收状态。

具体协议与限制见 [兼容性表](docs/COMPATIBILITY.md)。当前“按规则”默认仅绕过局域网，再匹配用户的直连/代理域名列表；没有捆绑中国 IP/地理规则数据库。系统代理只能影响遵守系统代理设置的应用。

## 当前验证边界

真实内核的单节点和两跳代理转发、流量计数、订阅更新保护、API 认证，以及界面连接/导入/手动填写测试已覆盖。本机已实际点击关闭窗口，确认 Flutter 进程退出后继续转发；经控制接口重开界面不重启连接，Command-Q 完整退出。打包冒烟还覆盖服务崩溃后的托盘恢复。

**TUN 的管理员授权、路由/DNS 清理和三平台系统代理实机验收尚未完成。** 这部分不能视为已验证的正式版能力。托盘菜单实际点击和 Windows/Linux 完整 GUI 仍需验收。详细记录见 [验证记录](docs/VALIDATION.md)。

订阅 URL、节点密码、私钥保存在当前用户的数据目录。数据库与备份没有应用层加密，备份可能含密码和订阅令牌，请妥善保管。默认不上传配置或遥测，不自动安装订阅指定的程序。

## 构建

工具链版本锁在 `core.lock.json`：Flutter 3.47.7 / Dart 3.13.5、Go 1.27.2、Rust 1.99.1。需启用 CGO，并安装目标系统的 C/C++ 编译工具。macOS 使用 Xcode；Windows 使用 Visual Studio C++ Desktop 以及 MinGW GCC（供 CGO）；Linux 依赖 GTK 3、Ayatana AppIndicator、Clang、CMake、Ninja 和 pkg-config。

在目标系统执行：

```sh
python3 scripts/build.py
```

产物位于 `dist/<系统>-<架构>/`。macOS 生成 `.app` 与 `.dmg`，Linux 生成 `.tar.gz` 和可用时的 `.deb`，Windows 生成便携 ZIP；Inno Setup 6 执行 `scripts/windows.iss` 可生成安装 EXE。GitHub Actions 已配置三平台构建。不要单独分发 Flutter 生成的 runner：用户入口应是打包后的原生托盘启动器。

```sh
cd service
# 与 NekoBox 相同的功能标签
go test -race -tags with_conntrack,with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api ./...
cd ../native/tray
cargo clippy --locked -- -D warnings
cd ../../apps/desktop
dart analyze lib test
flutter test --no-pub
```

当前 Flutter 的 `flutter analyze` 在含中文目录的此环境存在 LSP UTF-8 长度问题，使用 `dart analyze` 可正常分析，不需修改 SDK。

后台服务可独立运行：`shadowbox-service --data-dir <目录>`。开发和测试可设置 `SHADOWBOX_DATA_DIR`，与真实用户数据隔离。仅本地代理的打包冒烟测试：`python3 scripts/smoke_desktop.py dist/macos-arm64/ShadowBox.app`。

## 架构和开源

[开发约束](DEVELOPMENT.md) · [架构](docs/ARCHITECTURE.md) · [第三方声明](third_party/NOTICE.md)

项目使用 GPL-3.0-or-later。核心版本、提交和来源已固定，不使用 Clash 内核替换。FlClash 只作为界面参考，产品名称、图标和代码独立；未使用其商标。Android/iOS 尚未实现，未来需要单独接入平台 VPN 服务与生命周期。

公开源码仓库：[XZMoYing/ShadowBox](https://github.com/XZMoYing/ShadowBox)。构建产物、用户配置、数据库和开发缓存不提交到源码仓库。
