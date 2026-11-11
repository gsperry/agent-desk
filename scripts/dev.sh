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

url="http://localhost:5173/#t=$AGENT_DESK_TOKEN"
i=0
until curl -fs -o /dev/null http://localhost:5173/ || [ $i -ge 60 ]; do
  sleep 0.5
  i=$((i + 1))
done
echo "Open: $url"
if [ -z "${NO_OPEN:-}" ]; then
  case "$(uname)" in
    Darwin) open "$url" ;;
    *) command -v xdg-open >/dev/null && xdg-open "$url" ;;
  esac
fi
wait
