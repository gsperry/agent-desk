# Desktop app

Status: design only. Needs the owner's decisions below, and a newer Rust toolchain.

## What

A macOS app around Agent Desk: its own window and dock icon, notifications without a browser tab, a dock badge with the number of sessions that need you, a menu-bar count, and a global hotkey to bring it forward.

## Shape

- Tauri 2 shell in `desktop/`. The Go binary stays the whole app; the shell runs it as a sidecar (`agent-desk --no-open --port 0`), reads the URL with its token from stdout, and loads it in the webview. Quitting the app stops the sidecar and leaves every agent running.
- The web app detects the shell (`window.__TAURI__`) and, there only, sends notifications and the attention count through Tauri's notification and badge APIs instead of the browser's. In a browser nothing changes.
- The server's checks stay as they are: the webview loads `http://127.0.0.1:<port>`, so Host and Origin checks pass unchanged, and the token travels in the fragment as today.
- The global hotkey and menu-bar item come from Tauri's global-shortcut and tray APIs.

## Finding from a spike

Tauri 2's current dependencies do not build with the installed Rust 1.82: cargo fails to parse a dependency manifest (`cssparser-macros`), which needs edition 2024 support (Rust 1.85 or later). Building the shell needs `rustup update`. The spike lives outside the repository, in `/tmp/agentdesk-research/tauri-spike`.

## Decisions needed

1. Update Rust (`rustup update stable`) on this machine.
2. Signing and distribution: unsigned local builds only, or a Developer ID certificate and notarization for others to install.
3. Whether a Rust project belongs in this repository (`desktop/`) or in a separate one, since the rest of the build is Go and Node only.
