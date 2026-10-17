# Desktop client development brief

This document records the agreed product and engineering constraints for the first desktop release. It is the product baseline. The implementation record below distinguishes shipped code from verified platform behavior.

## Product scope

- First release: Windows, macOS, and Linux, distributed as standalone desktop packages.
- Future direction: Android and iOS. Keep the interface, shared proxy profile model, and protocol/configuration semantics reusable; each mobile operating system still needs its own VPN and lifecycle integration.
- The client follows NekoBox for Android behavior and directly builds the matching MatsuriDayo sing-box source. FlClash is a visual and interaction reference, not the proxy core.
- Keep the product name and visual identity independent of upstream projects. Plan an open-source release with compatible third-party license notices and source availability.

## Visual requirements

- The first-run default color scheme must not use blue or violet as its dominant/accent palette. Use a calm teal-green or warm neutral default. Do not allow operating-system dynamic colors to silently change that first-run default.
- Custom color settings may offer blue, indigo, violet, and other palettes; this restriction applies to the default only.
- Do not put oversized text, a large page title, subtitle, or slogan in the upper-left corner. Do not add marketing-style copy that serves no product task.
- Open directly to connection state and useful actions: connection status, selected node or chain, capture mode, routing mode, and traffic. Keep page labels and navigation compact.
- Use Material 3 foundations with an expressive but restrained component shape, typography, and motion system. Honor reduced-motion settings and support both comfortable and compact desktop density.
- Do not copy upstream logos, product names, or distinctive branding.

## Architecture

- Flutter and Dart render the foreground application. Closing its window saves view state and exits the Flutter process.
- A small Rust process owns the native tray and starts or focuses the interface on demand.
- A Go background service owns profiles, subscription scheduling, the local database, configuration generation, and the embedded NekoBox-compatible sing-box core. Use authenticated local IPC for control and status; packet traffic does not pass through Flutter or the tray.
- SQLite has one writer: the background service. The GUI is a client and can exit without affecting active proxy sessions.
- A small, OS-specific privileged helper may perform only the required TUN, route, DNS, and cleanup operations. Select handle passing versus a separate network worker after testing the pinned core on each operating system. Never run the Flutter UI elevated.
- Spawn external protocol plugins only when the selected profile needs them. A plugin package must be explicitly compatible with the operating system and CPU architecture.

### Foreground and background lifecycle

| State | Processes and behavior |
|---|---|
| Main window open | Flutter UI, tray, and background networking service; update only the visible views. |
| Window closed | Flutter process exits. The tray and service continue proxying and handling subscription schedules. Stop visual polling, charts, and detailed connection subscriptions. |
| Proxy paused | Tear down the active core instance and its plugins, restore settings owned by the client, and retain the tray for quick restart. |
| User quits | Restore client-owned network settings, stop the core and plugins, then exit the session processes. |

Tray actions should cover connect/pause, current node, favorites/recent nodes, capture and routing modes, subscription refresh, show window, and quit. Where a Linux desktop does not expose a supported tray, keep the application launcher as a documented way to reopen the GUI and define tested desktop-environment support.

## Core and compatibility baseline

At development start, select the latest official NekoBox release, then lock its release tag and exact commits for NekoBox, `MatsuriDayo/sing-box`, and `libneko`. Record the Go toolchain, build tags, and license notices too. Update this baseline deliberately and retain a rollback path; do not build from a floating branch.

The repository snapshot checked for this brief was NekoBox 1.4.2, which pins sing-box as `1.12.19-neko-1`. This is a research reference, not a permanent pin: re-check the latest release when implementation starts.

Compatibility should cover, against the selected baseline:

- Common share links and encoded node lists; Clash Meta, v2rayN, Shadowsocks, sing-box outbound, WireGuard files, and ZIP/file import.
- Manual profiles, subscription groups, refresh behavior, node selection and testing, sorting, search, duplicate handling, and export/backup.
- NekoBox-style proxy chains, preserving order, endpoint restrictions, and plugin requirements.
- External protocol plugins with per-OS/architecture availability, signing, version, startup, shutdown, and capability checks. Android APK plugins are not desktop plugins.
- Full sing-box configurations as an advanced import path. Document which GUI controls become unavailable for user-supplied configurations.
- `sn://` and NekoBox backups only against an explicitly tested version. Do not imply cross-version compatibility for this internal format.

Subscription import must preview recognized nodes and errors, preserve existing data on refresh failure, retain profile identity where possible, validate chain references before replacing profiles, and make selection changes visible. A newly selected node while connected may restart the session; show the running node separately from the stored selection.

## Network modes and safety behavior

Expose two separate choices: traffic capture (system proxy, TUN, or local proxy only) and routing policy (rule, global proxy, or direct). Default to one capture method at a time; simultaneous system proxy and TUN may be an advanced option.

Each desktop platform needs an explicit adapter for system proxy settings, TUN creation, routes, DNS, cleanup, network changes, sleep/wake, IPv4/IPv6, loop prevention, and local-network bypass. The system-proxy mode cannot claim to capture applications that ignore OS proxy settings.

Choose and document the default policy for a failed core: restore normal networking and allow direct traffic, or block proxy-bound traffic until the user recovers the service. Whichever is chosen, test crashes and forced termination to ensure routes and proxy settings are not left in an unknown state. Modify and restore only settings owned by this client.

## Performance and operating targets

Optimize for low idle cost. The UI process must be absent after its window is closed. Do not keep connection history, charts, or frequent status polling alive when the user is not viewing them. Avoid automatic full-subscription latency tests; bound their concurrency when requested. Release the core instance and external plugins when the proxy is paused.

Use initial targets of at most 120 MiB for an idle, closed-window session with one active built-in node and no external plugins, and 300 MiB with the main interface open. These are budgets to measure per platform, not measured guarantees. Record process-level memory, total client memory, and idle CPU separately for each operating system. Revisit the targets using real profiles, large rulesets, active connections, TUN, and plugins.

## Technology and build environment

- UI: Flutter stable and Dart; design system: Material 3 widgets plus a project-owned expressive theme layer.
- Shared native service and NekoBox-compatible core: Go.
- Tray/control shell: Rust, using native desktop integrations.
- Local control: authenticated IPv4 loopback HTTP/SSE, with a private discovery file; application data: SQLite, managed by the Go service.
- This Mac is the local macOS build/test host. Produce and verify Windows packages on a Windows runner and Linux packages on a Linux runner or native host; desktop targets do not share their platform SDKs or permission adapters.
- Pin Flutter, Go, and Rust versions for reproducible builds. Keep generated artifacts, signing configuration, platform setup, and dependency/license manifests in the project documentation.
- Current local host: macOS 14.7.4 on Apple Silicon; Xcode 16.2; Flutter 3.47.7 with Dart 3.13.5; Go 1.27.2; Rustup 1.29.1 with Rust stable 1.99.1, Clippy, and rustfmt; CMake 4.4.3; pkg-config 3.0.7; and Homebrew. Go was built from source because this machine's macOS version and Homebrew prefix did not have a matching binary package.
- Flutter can see the macOS desktop target. The existing Android SDK is missing command-line tools and accepted licenses. Xcode cannot currently enumerate simulator runtimes, and CocoaPods is absent; complete these before Android/iOS builds or when a selected macOS plugin requires them. Chrome is absent; web is outside the first-release scope. Refresh this inventory when the mobile phase begins.

## Suggested implementation gates

1. Confirm the current NekoBox release and lock its core commits and licenses.
2. Prove the background service and native tray can continue working after the Flutter process exits and can reopen the UI.
3. Prove system-proxy apply/restore and TUN apply/restore on Windows, macOS, and Linux, including unexpected termination.
4. Verify profile/subscription compatibility and chain validation against NekoBox fixtures before filling out the UI.
5. Measure the performance budgets on each supported OS and publish packages from native runners.

## 0.1.0 implementation record (2026-10-17)

Development was explicitly authorized by the user. The implementation uses Flutter `material_ui` for the foreground, Rust `tao`/`tray-icon` for the native launcher/tray, and a Go service embedding the exact pinned NekoBox fork. No FlClash engine/code is substituted. The latest official NekoBox release was rechecked as 1.4.2 and the resolved commits are in `core.lock.json`.

The selected desktop IPC is random-port loopback HTTP with a 192-bit Bearer token and SSE status events. Unix discovery/configuration files are private; Windows uses the user-profile directory ACL. This supersedes the earlier suggestion of OS-specific sockets/pipes. A separate elevated Go network worker implements TUN while Flutter, the tray, database, subscriptions, and plugins remain unprivileged.

Implemented: common protocol forms and links, the current-version sn decoder, node/subscription persistence, preview validation, proxy chains, external plugin launch adapters, system-proxy adapters, TUN worker, background lifecycle, theme settings, backup, and native packaging scripts. Current unsupported cases and platform limitations are normative in `docs/COMPATIBILITY.md`; full NekoBox database migration and arbitrary full-configuration mode remain future work.

Automated verification and measurements are recorded in `docs/VALIDATION.md`. A local Mac build is not evidence of Windows/Linux native runtime validation. Administrator-authorized TUN tests and real system-proxy mutation tests remain outstanding. Do not describe the preview as a fully validated three-platform stable release.

The visual rules remain mandatory: 默认不使用蓝紫配色，自定义颜色可以包含蓝紫选项；左上角不放大字、标题、副标题或无意义 slogan。
