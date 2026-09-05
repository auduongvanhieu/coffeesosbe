#!/usr/bin/env bash
# Deploy the backend + Postgres to a Docker host over SSH.
#   DEPLOY_HOST (ssh alias, default hector_vps)
#   DEPLOY_DIR  (remote dir, default /root/coffeesos)
# Note: the VPS runs Docker from snap, which can only read paths under /root,
# so the remote dir must live in the home directory (not /opt).
# The remote steps live in scripts/remote-up.sh so docker never competes with
# bash for stdin.
set -euo pipefail

HOST="${DEPLOY_HOST:-hector_vps}"
DIR="${DEPLOY_DIR:-/root/coffeesos}"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d%H%M)}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> syncing $ROOT -> $HOST:$DIR"
ssh "$HOST" "mkdir -p '$DIR'"
rsync -az --delete \
  --exclude .git --exclude .env --exclude bin --exclude '*.log' --exclude .DS_Store \
  "$ROOT/" "$HOST:$DIR/"

echo "==> building and starting on $HOST"
ssh "$HOST" "VERSION='$VERSION' bash '$DIR/scripts/remote-up.sh'"

PUBLIC_URL="${PUBLIC_URL:-https://dev.coffeesos.online/api}"
echo "==> done: $PUBLIC_URL/healthz"
curl -fsS --max-time 10 "$PUBLIC_URL/healthz" && echo || echo "    (public URL not reachable yet: check nginx / DNS)"
