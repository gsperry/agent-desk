# Desktop app

Status: built for macOS as unsigned local builds (`make desktop`).

## What

A macOS app around Agent Desk: its own window and dock icon, native notifications, the number of sessions that need you on the dock icon and in the menu bar, and Control-Option-A to bring it forward from anywhere.

## Shape

- Tauri 2 shell in `desktop/`. The Go binary stays the whole app. The shell bundles it as a sidecar, runs it through the user's login shell (`$SHELL -lc`) so it finds `agent-manager`, `tmux` and `code` on the same PATH as a terminal, reads the `Open:` line with the token from stdout, and loads that address in its window. Closing the window hides it, so notifications and counts keep coming; the dock icon, the menu-bar item and the shortcut bring it back. Quitting (menu bar or ⌘Q) stops the server and leaves every agent running.
- The login shell starts from a clean environment (HOME, USER, SHELL, TMPDIR and locale only), as a new terminal does, so variables from whatever launched the app never reach the server. Its PATH is the one an interactive terminal has, read once from `$SHELL -ilc` with a marker and a 5 second timeout, because installers often add their directory in `~/.zshrc`, which a login shell alone skips (Claude Code's `~/.local/bin` among them; without it agent-manager reports claude as not installed). The server's log goes to `~/Library/Logs/Agent Desk/server.log` (owner-only, restarted past 1 MB); the token only travels on stdout.
- The shell accepts only a `127.0.0.1` address from the server. Links to the web (PR links and other http or https links) open in the default browser through `open`; the window never leaves Agent Desk's own address, and other link schemes are dropped. The server's checks stay as they are: Host and Origin checks pass unchanged and the token travels in the fragment.
- The page gets two commands and nothing else (`desktop/capabilities/page.json`, remote URL `http://127.0.0.1:*`): `notify` (title and body, truncated) and `set_badge` (a count, shown on the dock icon and in the menu bar). The web app detects the shell (`window.__TAURI__`) and uses them in place of the browser's Notification API; in a browser nothing changes.
- The app runs its own server on port 7318 (a free one if that is taken), next to any `agent-desk` started from a terminal. The port stays the same between launches because the page keeps drafts, reading positions and settings per origin. Remote access is served by whichever started first; the other tries the socket every 15 seconds and takes over when the first stops.
- The server runs with `--exit-with-stdin` and the app holds its stdin open, so the server stops with the app however the app ends, a forced kill included.
- Settings → Desktop app → "Start at login, in the background" installs a Launch Agent (`tauri-plugin-autostart`) that starts the app with `--hidden`: no window, but remote access and notifications run after a restart. The page reads and changes it through two more commands, `autostart_get` and `autostart_set`.

## Limits

- Clicking a notification brings the app forward but does not open the session.
- The shortcut is fixed. If another app holds it, the app starts without it.
- Unsigned: macOS Gatekeeper blocks a copy downloaded from elsewhere. Builds are for the machine that made them.

## Next

- Signing and notarization, if the app is to be shared.
