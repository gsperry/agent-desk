#!/bin/sh
# Runs the Go backend under air and the Vite dev server, sharing one token
# so backend restarts keep the open page authorized.
set -eu
cd "$(dirname "$0")/.."

AGENT_DESK_TOKEN=$(openssl rand -hex 32)
export AGENT_DESK_TOKEN

trap 'kill 0' INT TERM EXIT

go run github.com/air-verse/air@v1.67.4 &
(cd web && npm run dev) &
vite=$!

# The token goes to whatever answers on the port, so wait for this script's
# Vite and refuse a server that some other process started.
i=0
until curl -fs -o /dev/null http://127.0.0.1:5173/; do
  if ! kill -0 "$vite" 2>/dev/null; then
    echo "Vite did not start. Is port 5173 in use?" >&2
    exit 1
  fi
  i=$((i + 1))
  if [ "$i" -ge 60 ]; then
    echo "Vite did not answer on 127.0.0.1:5173." >&2
    exit 1
  fi
  sleep 0.5
done
if command -v lsof >/dev/null; then
  listener=$(lsof -tnP -iTCP@127.0.0.1:5173 -sTCP:LISTEN | head -1)
  if [ "$(ps -o pgid= -p "$listener" | tr -d ' ')" != "$(ps -o pgid= -p $$ | tr -d ' ')" ]; then
    echo "Port 5173 is held by another process (pid $listener). Not sending it the token." >&2
    exit 1
  fi
fi

url="http://127.0.0.1:5173/#t=$AGENT_DESK_TOKEN"
echo "Open: $url"
if [ -z "${NO_OPEN:-}" ]; then
  # Open a private launcher file, not the URL, so the token never shows in
  # a process's arguments.
  case "$(uname)" in
    Darwin) cache="$HOME/Library/Caches/agent-desk" ;;
    *) cache="${XDG_CACHE_HOME:-$HOME/.cache}/agent-desk" ;;
  esac
  (umask 077 && mkdir -p "$cache")
  chmod 700 "$cache"
  launcher="$cache/dev-$$.html"
  (umask 077 && printf '<!doctype html><meta http-equiv="refresh" content="0;url=%s">\n' "$url" > "$launcher")
  case "$(uname)" in
    Darwin) open "$launcher" ;;
    *) command -v xdg-open >/dev/null && xdg-open "$launcher" ;;
  esac
  (sleep 30 && rm -f "$launcher") &
fi
wait
