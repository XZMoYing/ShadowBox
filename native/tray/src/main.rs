#![cfg_attr(target_os = "windows", windows_subsystem = "windows")]

use fs2::FileExt;
use serde::Deserialize;
use serde_json::{Value, json};
use std::{
    env,
    fs::{self, OpenOptions},
    io::{BufRead, BufReader},
    path::{Path, PathBuf},
    process::{Command, Stdio},
    sync::{
        Arc,
        atomic::{AtomicBool, Ordering},
    },
    thread,
    time::Duration,
};
use tao::{
    event::{Event, StartCause},
    event_loop::{ControlFlow, EventLoopBuilder},
};
use tray_icon::{
    Icon, TrayIcon, TrayIconBuilder, TrayIconEvent,
    menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu},
};

#[derive(Clone, Debug, Deserialize)]
struct Discovery {
    endpoint: String,
    token: String,
}
#[derive(Debug)]
enum Action {
    State(Value),
    Discovery(Discovery),
    Quit,
    MenuNodes(Value),
    Menu(String),
    Open,
    Error(String),
}
fn data_dir() -> PathBuf {
    env::var_os("SHADOWBOX_DATA_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            dirs::config_dir()
                .unwrap_or_else(env::temp_dir)
                .join("ShadowBox")
        })
}
fn binary(name: &str) -> PathBuf {
    #[cfg(target_os = "macos")]
    if name == "shadowbox-ui" {
        let helper = env::current_exe()
            .unwrap()
            .parent()
            .unwrap()
            .parent()
            .unwrap()
            .join("Helpers/ShadowBox UI.app/Contents/MacOS/shadowbox-ui");
        if helper.exists() {
            return helper;
        }
    }
    let file = if cfg!(windows) {
        format!("{name}.exe")
    } else {
        name.to_owned()
    };
    env::current_exe().unwrap().parent().unwrap().join(file)
}
fn discovery(dir: &Path) -> Result<Discovery, Box<dyn std::error::Error>> {
    Ok(serde_json::from_slice(&fs::read(
        dir.join("service.json"),
    )?)?)
}
fn client() -> ureq::Agent {
    ureq::AgentBuilder::new()
        .timeout_connect(Duration::from_secs(3))
        .timeout_read(Duration::from_secs(150))
        .build()
}
fn api(d: &Discovery, method: &str, path: &str, body: Value) -> Result<Value, String> {
    let request = client()
        .request(method, &format!("{}{}", d.endpoint, path))
        .set("Authorization", &format!("Bearer {}", d.token));
    let result = if method == "GET" || method == "DELETE" {
        request.call()
    } else {
        request.send_json(body)
    };
    match result {
        Ok(r) => r.into_json().map_err(|e| e.to_string()),
        Err(ureq::Error::Status(_, r)) => {
            let v: Value = r.into_json().unwrap_or(Value::Null);
            Err(v["error"]
                .as_str()
                .unwrap_or("后台服务请求失败")
                .to_string())
        }
        Err(e) => Err(e.to_string()),
    }
}
fn command(path: impl AsRef<std::ffi::OsStr>) -> Command {
    let mut cmd = Command::new(path);
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(0x08000000);
    }
    #[cfg(not(windows))]
    let _ = &mut cmd;
    cmd
}
fn service(dir: &Path) -> Result<Discovery, Box<dyn std::error::Error>> {
    if let Ok(d) = discovery(dir)
        && api(&d, "GET", "/v1/status", Value::Null).is_ok()
    {
        return Ok(d);
    };
    let log_path = dir.join("service.log");
    if fs::metadata(&log_path).is_ok_and(|m| m.len() > 1_048_576) {
        fs::write(&log_path, [])?
    };
    let log = OpenOptions::new()
        .create(true)
        .append(true)
        .open(log_path)?;
    let path = env::var_os("SHADOWBOX_SERVICE")
        .map(PathBuf::from)
        .unwrap_or_else(|| binary("shadowbox-service"));
    let mut process = command(path)
        .arg("--data-dir")
        .arg(dir)
        .current_dir(dir)
        .stdout(Stdio::from(log.try_clone()?))
        .stderr(Stdio::from(log))
        .spawn()?;
    thread::spawn(move || {
        let _ = process.wait();
    });
    for _ in 0..100 {
        thread::sleep(Duration::from_millis(100));
        if let Ok(d) = discovery(dir)
            && api(&d, "GET", "/v1/status", Value::Null).is_ok()
        {
            return Ok(d);
        }
    }
    Err("后台服务启动超时，请查看 service.log".into())
}
fn icon(active: bool) -> Icon {
    let size = 32;
    let mut rgba = vec![0; size * size * 4];
    for y in 0..size {
        for x in 0..size {
            let dx = x as f64 - 15.5;
            let dy = y as f64 - 15.5;
            let r = (dx * dx + dy * dy).sqrt();
            if (9.0..13.0).contains(&r) || (dx.abs() < 2.0 && dy < 0.0 && dy > -13.0) {
                let i = (y * size + x) * 4;
                let c = if active {
                    [16, 112, 86, 255]
                } else {
                    [100, 110, 104, 255]
                };
                rgba[i..i + 4].copy_from_slice(&c)
            }
        }
    }
    Icon::from_rgba(rgba, size as u32, size as u32).unwrap()
}
struct Ui {
    pid: u32,
    running: Arc<AtomicBool>,
}
impl Ui {
    fn stop(&self) {
        if !self.running.load(Ordering::Acquire) {
            return;
        }
        #[cfg(unix)]
        let _ = Command::new("/bin/kill")
            .args(["-TERM", &self.pid.to_string()])
            .status();
        #[cfg(windows)]
        let _ = command("taskkill.exe")
            .args(["/PID", &self.pid.to_string(), "/T", "/F"])
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();
    }
}
fn show_ui(child: &mut Option<Ui>, dir: &Path) -> Result<(), String> {
    if child
        .as_ref()
        .is_some_and(|p| p.running.load(Ordering::Acquire))
    {
        return Ok(());
    }
    let path = env::var_os("SHADOWBOX_UI")
        .map(PathBuf::from)
        .unwrap_or_else(|| binary("shadowbox-ui"));
    let log_path = dir.join("ui.log");
    if fs::metadata(&log_path).is_ok_and(|m| m.len() > 1_048_576) {
        fs::write(&log_path, []).map_err(|e| e.to_string())?;
    }
    let log = OpenOptions::new()
        .create(true)
        .append(true)
        .open(log_path)
        .map_err(|e| e.to_string())?;
    let mut process = command(path)
        .env("SHADOWBOX_DATA_DIR", dir)
        .stdout(Stdio::from(log.try_clone().map_err(|e| e.to_string())?))
        .stderr(Stdio::from(log))
        .spawn()
        .map_err(|e| format!("界面启动失败：{e}"))?;
    let running = Arc::new(AtomicBool::new(true));
    *child = Some(Ui {
        pid: process.id(),
        running: running.clone(),
    });
    thread::spawn(move || {
        let _ = process.wait();
        running.store(false, Ordering::Release);
    });
    Ok(())
}
fn main() {
    if env::args().any(|a| a == "--version") {
        println!("ShadowBox tray 0.1.0");
        return;
    };
    if let Err(e) = run() {
        eprintln!("ShadowBox: {e}");
        let _ = fs::write(data_dir().join("tray-error.log"), e.to_string());
    }
}
fn run() -> Result<(), Box<dyn std::error::Error>> {
    let dir = data_dir();
    fs::create_dir_all(&dir)?;
    let lock = OpenOptions::new()
        .create(true)
        .truncate(false)
        .read(true)
        .write(true)
        .open(dir.join("tray.lock"))?;
    if lock.try_lock_exclusive().is_err() {
        if let Ok(d) = discovery(&dir) {
            let _ = api(&d, "POST", "/v1/ui/show", json!({}));
        }
        return Ok(());
    }
    let mut d = service(&dir)?;
    let initial_show = !env::args().any(|s| s == "--background");
    #[allow(unused_mut)]
    let mut event_loop = EventLoopBuilder::<Action>::with_user_event().build();
    #[cfg(target_os = "macos")]
    {
        use tao::platform::macos::{ActivationPolicy, EventLoopExtMacOS};
        event_loop.set_activation_policy(ActivationPolicy::Accessory);
    }
    let proxy = event_loop.create_proxy();
    let mp = proxy.clone();
    MenuEvent::set_event_handler(Some(move |e: MenuEvent| {
        let _ = mp.send_event(Action::Menu(e.id.0));
    }));
    let tp = proxy.clone();
    TrayIconEvent::set_event_handler(Some(move |e: TrayIconEvent| {
        if matches!(e, TrayIconEvent::DoubleClick { .. }) {
            let _ = tp.send_event(Action::Open);
        }
    }));
    let watcher = proxy.clone();
    let mut wd = d.clone();
    let watch_dir = dir.clone();
    thread::spawn(move || {
        loop {
            let result = client()
                .get(&format!("{}/v1/events", wd.endpoint))
                .set("Authorization", &format!("Bearer {}", wd.token))
                .call();
            if let Ok(r) = result {
                for line in BufReader::new(r.into_reader()).lines() {
                    match line {
                        Ok(s) => {
                            if let Some(s) = s.strip_prefix("data: ")
                                && let Ok(v) = serde_json::from_str(s)
                                && watcher.send_event(Action::State(v)).is_err()
                            {
                                return;
                            }
                        }
                        Err(_) => break,
                    }
                }
            };
            thread::sleep(Duration::from_secs(2));
            match service(&watch_dir) {
                Ok(next) => {
                    wd = next.clone();
                    if watcher.send_event(Action::Discovery(next)).is_err() {
                        return;
                    }
                }
                Err(e) => {
                    if watcher
                        .send_event(Action::Error(format!("后台服务恢复失败：{e}")))
                        .is_err()
                    {
                        return;
                    }
                }
            }
        }
    });
    let menu = Menu::new();
    let state_label = MenuItem::with_id("status", "未连接", false, None);
    let show = MenuItem::with_id("show", "打开 ShadowBox", true, None);
    let toggle = MenuItem::with_id("toggle", "连接", true, None);
    let nodes_menu = Submenu::new("节点", true);
    let capture = Submenu::new("接管方式", true);
    for (id, label) in [
        ("capture:system", "系统代理"),
        ("capture:tun", "TUN 虚拟网卡"),
        ("capture:manual", "仅本地代理"),
    ] {
        capture.append(&MenuItem::with_id(id, label, true, None))?
    }
    let routing = Submenu::new("分流策略", true);
    for (id, label) in [
        ("routing:rule", "按规则"),
        ("routing:global", "全局代理"),
        ("routing:direct", "全局直连"),
    ] {
        routing.append(&MenuItem::with_id(id, label, true, None))?
    }
    let refresh = MenuItem::with_id("refresh", "更新订阅", true, None);
    let quit = MenuItem::with_id("quit", "退出并恢复网络", true, None);
    menu.append_items(&[
        &state_label,
        &PredefinedMenuItem::separator(),
        &show,
        &toggle,
        &nodes_menu,
        &capture,
        &routing,
        &refresh,
        &PredefinedMenuItem::separator(),
        &quit,
    ])?;
    let mut tray: Option<TrayIcon> = None;
    let mut child: Option<Ui> = None;
    let mut state = json!({});
    let mut quitting = false;
    let mut wake = 0u64;
    let mut shown = false;
    let mut node_items: Vec<MenuItem> = vec![];
    event_loop.run(move |event, _, flow| {
        *flow = ControlFlow::Wait;
        let _keep_lock = &lock;
        match event {
            Event::NewEvents(StartCause::Init) => {
                tray = TrayIconBuilder::new()
                    .with_menu(Box::new(menu.clone()))
                    .with_tooltip("ShadowBox")
                    .with_icon(icon(false))
                    .build()
                    .ok();
                if initial_show && !shown {
                    shown = true;
                    if let Err(e) = show_ui(&mut child, &dir) {
                        state_label.set_text(e)
                    }
                }
            }
            Event::UserEvent(Action::State(v)) => {
                let running = v["running"].as_bool().unwrap_or(false);
                let busy = v["busy"].as_bool().unwrap_or(false);
                let name = v["active_name"].as_str().unwrap_or("");
                let error = v["error"].as_str().unwrap_or("");
                state_label.set_text(if busy {
                    "正在处理…".to_string()
                } else if !error.is_empty() {
                    error.to_string()
                } else if running {
                    format!("已连接 · {name}")
                } else {
                    "未连接".to_string()
                });
                toggle.set_text(if running { "暂停代理" } else { "连接" });
                toggle.set_enabled(!busy);
                if let Some(t) = &tray {
                    let _ = t.set_icon(Some(icon(running)));
                    let _ = t.set_tooltip(Some(if running {
                        format!("ShadowBox · {name}")
                    } else {
                        "ShadowBox · 未连接".into()
                    }));
                };
                let next = v["wake_generation"].as_u64().unwrap_or(0);
                if next > wake
                    && let Err(e) = show_ui(&mut child, &dir)
                {
                    state_label.set_text(e)
                };
                wake = next;
                if v["quit_requested"].as_bool().unwrap_or(false) && !quitting {
                    quitting = true;
                    let dd = d.clone();
                    let pp = proxy.clone();
                    thread::spawn(move || match api(&dd, "POST", "/v1/shutdown", json!({})) {
                        Ok(_) => {
                            let _ = pp.send_event(Action::Quit);
                        }
                        Err(e) => {
                            let _ = pp.send_event(Action::Error(e));
                        }
                    });
                }
                state = v;
                let nd = d.clone();
                let np = proxy.clone();
                thread::spawn(move || {
                    if let Ok(v) = api(&nd, "GET", "/v1/menu", Value::Null) {
                        let _ = np.send_event(Action::MenuNodes(v));
                    }
                });
            }
            Event::UserEvent(Action::MenuNodes(v)) => {
                for item in node_items.drain(..) {
                    let _ = nodes_menu.remove(&item);
                }
                if let Some(nodes) = v.as_array() {
                    for n in nodes {
                        let id = n["id"].as_str().unwrap_or("");
                        let name = n["name"].as_str().unwrap_or("节点");
                        let item = MenuItem::with_id(format!("node:{id}"), name, true, None);
                        let _ = nodes_menu.append(&item);
                        node_items.push(item);
                    }
                }
            }
            Event::Reopen { .. } | Event::UserEvent(Action::Open) => {
                let dd = d.clone();
                thread::spawn(move || {
                    let _ = api(&dd, "POST", "/v1/ui/show", json!({}));
                });
            }
            Event::UserEvent(Action::Discovery(next)) => {
                d = next;
                wake = 0;
            }
            Event::UserEvent(Action::Quit) => {
                if let Some(p) = child.as_mut() {
                    p.stop();
                }
                *flow = ControlFlow::Exit;
            }
            Event::UserEvent(Action::Error(e)) => {
                quitting = false;
                state_label.set_text(e);
            }
            Event::UserEvent(Action::Menu(id)) => {
                if id == "show" {
                    let dd = d.clone();
                    thread::spawn(move || {
                        let _ = api(&dd, "POST", "/v1/ui/show", json!({}));
                    });
                } else {
                    let dd = d.clone();
                    let pp = proxy.clone();
                    let running = state["running"].as_bool().unwrap_or(false);
                    let selected = state["selected"].as_str().unwrap_or("").to_string();
                    if id == "quit" {
                        quitting = true;
                        toggle.set_enabled(false);
                        state_label.set_text("正在停止代理并恢复网络…");
                        thread::spawn(move || match api(&dd, "POST", "/v1/shutdown", json!({})) {
                            Ok(_) => {
                                let _ = pp.send_event(Action::Quit);
                            }
                            Err(e) => {
                                let _ = pp.send_event(Action::Error(e));
                            }
                        });
                    } else {
                        thread::spawn(move || {
                            let result = if id == "toggle" {
                                api(
                                    &dd,
                                    "POST",
                                    if running { "/v1/stop" } else { "/v1/connect" },
                                    json!({"id":selected}),
                                )
                            } else if let Some(id) = id.strip_prefix("node:") {
                                api(&dd, "POST", "/v1/select", json!({"id":id}))
                            } else if id.starts_with("capture:") || id.starts_with("routing:") {
                                api(&dd, "GET", "/v1/settings", Value::Null).and_then(|mut s| {
                                    let (key, value) = id.split_once(':').unwrap();
                                    s[key] = json!(value);
                                    api(&dd, "POST", "/v1/settings", s)
                                })
                            } else if id == "refresh" {
                                api(&dd, "GET", "/v1/subscriptions", Value::Null).and_then(|v| {
                                    for s in v.as_array().unwrap_or(&vec![]) {
                                        if let Some(id) = s["id"].as_str() {
                                            api(
                                                &dd,
                                                "POST",
                                                &format!("/v1/subscriptions/{id}/refresh"),
                                                json!({}),
                                            )?;
                                        }
                                    }
                                    Ok(json!({}))
                                })
                            } else {
                                Ok(json!({}))
                            };
                            if let Err(e) = result {
                                let _ = pp.send_event(Action::Error(e));
                            }
                        });
                    }
                }
            }
            _ => {}
        }
    });
}
