// Agent Desk as a macOS app. It starts the bundled agent-desk server on a
// free 127.0.0.1 port, opens a window on the address it prints, and stops
// the server when the app quits. Agents keep running in agent-manager.
//
// The page may call two commands, granted in capabilities/page.json:
// notify (a native notification, which the web view cannot show) and
// set_badge (the dock badge).

use std::io::{BufRead, BufReader};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use tauri::{Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_notification::NotificationExt;

struct Server(Mutex<Option<Child>>);

#[tauri::command]
fn notify(app: tauri::AppHandle, title: String, body: String) -> Result<(), String> {
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
fn set_badge(window: tauri::WebviewWindow, count: i64) -> Result<(), String> {
    let badge = if count > 0 { Some(count.min(999)) } else { None };
    window.set_badge_count(badge).map_err(|e| e.to_string())
}

/// start runs the bundled server through the user's login shell, so it sees
/// the same PATH (agent-manager, tmux, code) as in a terminal; an app opened
/// from Finder otherwise gets a minimal PATH. It returns the server and the
/// address with its token.
fn start() -> Result<(Child, String), String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    let server = exe
        .parent()
        .ok_or("no app directory")?
        .join("agent-desk");
    let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".into());
    let mut child = Command::new(shell)
        .arg("-lc")
        .arg(r#"exec "$0" --no-open --port 0"#)
        .arg(&server)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
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

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_notification::init())
        .manage(Server(Mutex::new(None)))
        .invoke_handler(tauri::generate_handler![notify, set_badge])
        .setup(|app| {
            let (child, url) = start().map_err(|e| Box::<dyn std::error::Error>::from(e))?;
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
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("could not build Agent Desk");
    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            if let Some(mut child) = handle.state::<Server>().0.lock().unwrap().take() {
                let _ = child.kill();
            }
        }
    });
}
