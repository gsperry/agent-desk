// Agent Desk as a macOS app. It starts the bundled agent-desk server on a
// free 127.0.0.1 port, opens a window on the address it prints, and stops
// the server when the app quits. Agents keep running in agent-manager.
//
// Closing the window hides it, so notifications and the counts keep
// working; the dock icon, the menu-bar item and Control-Option-A bring it
// back. The page may call two commands, granted in capabilities/page.json:
// notify (a native notification, which the web view cannot show) and
// set_badge (the dock badge and the menu-bar count).

use std::io::{BufRead, BufReader};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, WindowEvent};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};
use tauri_plugin_notification::NotificationExt;

struct Server(Mutex<Option<Child>>);

const TRAY: &str = "main";

#[tauri::command]
fn notify(app: AppHandle, title: String, body: String) -> Result<(), String> {
    let title: String = title.chars().take(200).collect();
    let body: String = body.chars().take(500).collect();
    app.notification()
        .builder()
        .title(title)
        .body(body)
        .show()
        .map_err(|e| e.to_string())
}

#[tauri::command]
fn set_badge(app: AppHandle, window: tauri::WebviewWindow, count: i64) -> Result<(), String> {
    let count = count.clamp(0, 999);
    let badge = (count > 0).then_some(count);
    if let Some(tray) = app.tray_by_id(TRAY) {
        let _ = tray.set_title(badge.map(|n| n.to_string()));
    }
    window.set_badge_count(badge).map_err(|e| e.to_string())
}

/// show brings the window forward, unhiding it if it was closed.
fn show(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

/// clean_shell is the user's shell with a clean environment, as a new
/// terminal starts, so nothing from whatever launched the app (a tmux
/// session, an agent's variables) reaches the server.
fn clean_shell(shell: &str) -> Command {
    let mut cmd = Command::new(shell);
    cmd.env_clear()
        .env("SHELL", shell)
        .env("PATH", "/usr/bin:/bin:/usr/sbin:/sbin");
    for key in [
        "HOME", "USER", "LOGNAME", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE",
    ] {
        if let Some(v) = std::env::var_os(key) {
            cmd.env(key, v);
        }
    }
    cmd
}

/// terminal_path is the PATH an interactive terminal has. A login shell
/// alone skips ~/.zshrc, where installers often add their directory (Claude
/// Code's ~/.local/bin among them), so the shell is run interactively once,
/// with a marker around the value and a timeout, in case the rc file
/// prints, prompts or hangs.
fn terminal_path(shell: &str) -> Option<String> {
    const MARK: &str = "__AGENT_DESK_PATH__";
    let mut child = clean_shell(shell)
        .arg("-ilc")
        .arg(format!(
            r#"printf '
{MARK}%s{MARK}
' "$PATH""#
        ))
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .ok()?;
    let mut out = child.stdout.take()?;
    let (tx, rx) = std::sync::mpsc::channel();
    std::thread::spawn(move || {
        let mut s = String::new();
        let _ = std::io::Read::read_to_string(&mut out, &mut s);
        let _ = tx.send(s);
    });
    let got = rx.recv_timeout(Duration::from_secs(5));
    let _ = child.kill();
    let _ = child.wait();
    let text = got.ok()?;
    let start = text.find(MARK)? + MARK.len();
    let end = start + text[start..].find(MARK)?;
    let path = text[start..end].trim();
    (!path.is_empty()).then(|| path.to_string())
}

/// start runs the bundled server through the user's login shell, with the
/// PATH an interactive terminal has, so it finds agent-manager, tmux, code
/// and the agent CLIs (claude) as a terminal would; an app opened from
/// Finder otherwise gets a minimal PATH. The shell starts from a clean
/// environment. The
/// server's log goes to ~/Library/Logs/Agent Desk/server.log. It returns
/// the server and the address with its token, which only stdout carries.
/// The port is fixed (7318, next to the terminal default 7317) because the
/// page's storage (drafts, reading positions, pins, settings) belongs to its
/// origin, port included; a busy port still falls back to a free one.
fn start() -> Result<(Child, String), String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    let server = exe.parent().ok_or("no app directory")?.join("agent-desk");
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".into());
    let mut cmd = clean_shell(&shell);
    // The login profile runs again and may add to this PATH; it keeps what
    // the interactive shell found.
    if let Some(path) = terminal_path(&shell) {
        cmd.env("AGENT_DESK_PATH", path);
    }
    let log = server_log()
        .map(Stdio::from)
        .unwrap_or_else(|_| Stdio::null());
    let mut child = cmd
        .arg("-lc")
        .arg(r#"[ -n "$AGENT_DESK_PATH" ] && PATH="$AGENT_DESK_PATH" && unset AGENT_DESK_PATH; exec "$0" --no-open --port 7318"#)
        .arg(&server)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(log)
        .spawn()
        .map_err(|e| format!("could not start {}: {e}", server.display()))?;
    let stdout = child.stdout.take().ok_or("no server output")?;
    let (tx, rx) = std::sync::mpsc::channel();
    std::thread::spawn(move || {
        for line in BufReader::new(stdout).lines().map_while(Result::ok) {
            if let Some(url) = line.strip_prefix("Open: ") {
                let _ = tx.send(url.trim().to_string());
            }
        }
    });
    match rx.recv_timeout(Duration::from_secs(30)) {
        Ok(url) => Ok((child, url)),
        Err(_) => {
            let _ = child.kill();
            Err("the Agent Desk server did not start within 30 seconds".into())
        }
    }
}

/// server_log opens the server's log, owner-only, starting it afresh past
/// 1 MB so it cannot grow without bound.
fn server_log() -> std::io::Result<std::fs::File> {
    use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
    let home = std::env::var_os("HOME").ok_or(std::io::ErrorKind::NotFound)?;
    let dir = std::path::Path::new(&home).join("Library/Logs/Agent Desk");
    std::fs::DirBuilder::new()
        .recursive(true)
        .mode(0o700)
        .create(&dir)?;
    let path = dir.join("server.log");
    let fresh = std::fs::metadata(&path).is_ok_and(|m| m.len() > 1 << 20);
    std::fs::OpenOptions::new()
        .create(true)
        .append(!fresh)
        .write(true)
        .truncate(fresh)
        .mode(0o600)
        .open(path)
}

fn tray(app: &AppHandle) -> tauri::Result<()> {
    let open = MenuItem::with_id(app, "show", "Show Agent Desk", true, Some("Ctrl+Alt+A"))?;
    let quit = MenuItem::with_id(app, "quit", "Quit Agent Desk", true, None::<&str>)?;
    let menu = Menu::with_items(app, &[&open, &PredefinedMenuItem::separator(app)?, &quit])?;
    TrayIconBuilder::with_id(TRAY)
        .icon(tauri::include_image!("icons/tray.png"))
        .icon_as_template(true)
        .tooltip("Agent Desk")
        .menu(&menu)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "show" => show(app),
            "quit" => app.exit(0),
            _ => {}
        })
        .build(app)?;
    Ok(())
}

fn main() {
    let shortcut = Shortcut::new(Some(Modifiers::CONTROL | Modifiers::ALT), Code::KeyA);
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(move |app, s, event| {
                    if *s == shortcut && event.state() == ShortcutState::Pressed {
                        show(app);
                    }
                })
                .build(),
        )
        .manage(Server(Mutex::new(None)))
        .invoke_handler(tauri::generate_handler![notify, set_badge])
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .setup(move |app| {
            let (child, url) = start().map_err(Box::<dyn std::error::Error>::from)?;
            *app.state::<Server>().0.lock().unwrap() = Some(child);
            let url: tauri::Url = url.parse()?;
            if url.host_str() != Some("127.0.0.1") {
                return Err("the server printed an unexpected address".into());
            }
            WebviewWindowBuilder::new(app, "main", WebviewUrl::External(url))
                .title("Agent Desk")
                .inner_size(1320.0, 860.0)
                .min_inner_size(420.0, 500.0)
                .build()?;
            tray(app.handle())?;
            // Another app may own the shortcut; the menu-bar item still works.
            if let Err(e) = app.global_shortcut().register(shortcut) {
                eprintln!("agent-desk: Control-Option-A is taken: {e}");
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("could not build Agent Desk");
    app.run(|handle, event| match event {
        RunEvent::Reopen { .. } => show(handle),
        RunEvent::Exit => {
            if let Some(mut child) = handle.state::<Server>().0.lock().unwrap().take() {
                let _ = child.kill();
            }
        }
        _ => {}
    });
}
