# Desktop app

Status: built for macOS as unsigned local builds (`make desktop`). Menu-bar count and global hotkey are next.

## What

A macOS app around Agent Desk: its own window and dock icon, native notifications, and a dock badge with the number of sessions that need you.

## Shape

- Tauri 2 shell in `desktop/`. The Go binary stays the whole app. The shell bundles it as a sidecar, runs it through the user's login shell (`$SHELL -lc`) so it finds `agent-manager`, `tmux` and `code` on the same PATH as a terminal, reads the `Open:` line with the token from stdout, and loads that address in its window. Quitting the app stops the server and leaves every agent running.
- The shell accepts only a `127.0.0.1` address from the server. The server's checks stay as they are: Host and Origin checks pass unchanged and the token travels in the fragment.
- The page gets two commands and nothing else (`desktop/capabilities/page.json`, remote URL `http://127.0.0.1:*`): `notify` (title and body, truncated) and `set_badge` (a count). The web app detects the shell (`window.__TAURI__`) and uses them in place of the browser's Notification API; in a browser nothing changes.
- The app runs its own server on a free port, next to any `agent-desk` started from a terminal. Remote access is served by whichever started first; the other reports the socket as in use.

## Limits

- Clicking a notification brings the app forward but does not open the session.
- Unsigned: macOS Gatekeeper blocks a copy downloaded from elsewhere. Builds are for the machine that made them.

## Next

- Menu-bar count and a global hotkey (Tauri tray and global-shortcut plugins).
- Signing and notarization, if the app is to be shared.
