#!/usr/bin/env bash
# Runs ON the Docker host (invoked by scripts/deploy.sh). Expects to be executed
# from the project directory. Idempotent: safe to run on every deploy.
#   VERSION  build tag baked into the binary (optional)
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  cat > .env <<ENV
APP_ENV=production
POSTGRES_PASSWORD=$(openssl rand -hex 24)
JWT_SECRET=$(openssl rand -hex 32)
API_BIND=0.0.0.0
API_PORT=8080
CORS_ORIGINS=*
SEED_ADMIN_EMAIL=admin@coffeesos.local
SEED_ADMIN_PASSWORD=$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-20)
SEED_DEMO=true
ENV
  chmod 600 .env
  echo "==> generated .env with fresh secrets (keep it: it holds the DB password and JWT secret)"
fi

export VERSION="${VERSION:-dev}"
docker compose build --pull api </dev/null
docker compose up -d </dev/null

echo "==> waiting for API health"
healthy=0
for _ in $(seq 1 30); do
  if docker compose exec -T api wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 </dev/null; then
    healthy=1
    break
  fi
  sleep 2
done
if [ "$healthy" != 1 ]; then
  echo "API did not become healthy" >&2
  docker compose logs --tail=50 api </dev/null
  exit 1
fi
echo "    healthy"

echo "==> seeding (idempotent: skips what already exists)"
docker compose run --rm -T api seed </dev/null

docker compose ps </dev/null
docker image prune -f >/dev/null 2>&1 </dev/null || true
