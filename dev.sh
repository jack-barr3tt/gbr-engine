#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$ROOT/.env"
SESSION="gbr-dev"
NO_ATTACH=false
[[ "${1:-}" == "--no-attach" ]] && NO_ATTACH=true

check_cmd() {
  command -v "$1" &>/dev/null || { echo "ERROR: $1 not found." >&2; exit 1; }
}
check_cmd tmux
check_cmd atlas
check_cmd air
check_cmd go

[[ -f "$ENV_FILE" ]] || { echo "ERROR: $ENV_FILE not found." >&2; exit 1; }

set -a
# shellcheck source=/dev/null
source "$ENV_FILE"
set +a

if tmux has-session -t "$SESSION" 2>/dev/null; then
  $NO_ATTACH && exit 0
  exec tmux attach-session -t "$SESSION"
fi

psql -d postgres -c "CREATE DATABASE gbr_engine_atlas_dev OWNER $POSTGRES_USER;" 2>/dev/null || true
(cd "$ROOT" && atlas schema apply --env local)

SETUP="set -a && source \"$ENV_FILE\" && set +a && export GOWORK=off REDIS_ADDR=127.0.0.1:6379 MQ_HOST=127.0.0.1 MQ_PORT=5672 PORT=3000"

declare -a SERVICES=(
  "http-api:api"
  "queuer:queuer"
  "trust-consumer:trust"
  "vstp-consumer:vstp"
  "data-fetcher:fetcher"
  "schedule-initializer:schedule"
)

tmux new-session -d -s "$SESSION" -n "${SERVICES[0]##*:}"
tmux send-keys -t "$SESSION:${SERVICES[0]##*:}" "$SETUP && cd \"$ROOT/src/${SERVICES[0]%%:*}\" && air" Enter

for entry in "${SERVICES[@]:1}"; do
  dir="${entry%%:*}"
  win="${entry##*:}"
  tmux new-window -t "$SESSION" -n "$win"
  tmux send-keys -t "$SESSION:$win" "$SETUP && cd \"$ROOT/src/$dir\" && air" Enter
done

$NO_ATTACH && exit 0
echo "Session $SESSION (api, queuer, trust, vstp, fetcher, schedule). Attaching..."
exec tmux attach-session -t "$SESSION"
