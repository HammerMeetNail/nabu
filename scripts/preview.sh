#!/usr/bin/env bash
# Run this checkout as a local preview in a Podman pod and publish it to your
# tailnet with Tailscale Serve, so you can use it from your phone.
#
#   scripts/preview.sh up            build this checkout and (re)start the preview
#   scripts/preview.sh down          stop it (keeps its database)
#   scripts/preview.sh status        show containers and URLs
#   scripts/preview.sh logs          follow the app log
#   scripts/preview.sh reset --yes   stop it and delete its database
#
# Ports bind to 127.0.0.1 only. Tailscale Serve publishes them over HTTPS to
# your tailnet, never the public internet. Override defaults with the
# NABU_PREVIEW_* variables below; NABU_PREVIEW_TAILSCALE=0 skips Tailscale.
set -euo pipefail

# Build the checkout this is run from (any worktree), falling back to the one
# that contains this script.
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || (CDPATH= cd -- "$(dirname -- "$0")/.." && pwd))
[ -f "$ROOT/cmd/vapid-keygen/main.go" ] || { echo "preview: run this from a Nabu checkout" >&2; exit 1; }
NAME=${NABU_PREVIEW_NAME:-nabu-preview}
APP_PORT=${NABU_PREVIEW_PORT:-18080}
MAIL_PORT=${NABU_PREVIEW_MAIL_PORT:-18025}
HTTPS_PORT=${NABU_PREVIEW_HTTPS_PORT:-18443}
MAIL_HTTPS_PORT=${NABU_PREVIEW_MAIL_HTTPS_PORT:-18444}
USE_TAILSCALE=${NABU_PREVIEW_TAILSCALE:-1}
ENV_FILE=${NABU_PREVIEW_ENV:-${XDG_CONFIG_HOME:-$HOME/.config}/nabu/preview.env}
GO=${GO:-go}
IMAGE=localhost/$NAME:latest
VOLUME=$NAME-pgdata

die() { echo "preview: $*" >&2; exit 1; }

# The machine's MagicDNS name (e.g. mbp16.tail1234.ts.net), or nothing.
tailnet_host() {
  [ "$USE_TAILSCALE" = 1 ] || return 0
  command -v tailscale >/dev/null 2>&1 || return 0
  tailscale status --self --json 2>/dev/null |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["Self"]["DNSName"].rstrip("."))' 2>/dev/null || true
}

# Web Push needs a VAPID key pair; generate one per machine, outside the repo.
ensure_env() {
  [ -f "$ENV_FILE" ] && return 0
  mkdir -p "$(dirname "$ENV_FILE")"
  echo "Generating Web Push (VAPID) keys in $ENV_FILE"
  local keys
  keys=$(cd "$ROOT" && "$GO" run ./cmd/vapid-keygen) || die "could not run cmd/vapid-keygen (set GO=/path/to/go)"
  (
    umask 077
    {
      echo "# Nabu local preview settings (not committed). Delete this file to regenerate."
      grep -E '^VAPID_(PRIVATE|PUBLIC)_KEY=' <<<"$keys"
      echo "VAPID_SUBJECT=mailto:preview@nabu.local"
    } >"$ENV_FILE"
  )
}

serve_on() {
  tailscale serve --bg --https="$HTTPS_PORT" "http://127.0.0.1:$APP_PORT" >/dev/null
  tailscale serve --bg --https="$MAIL_HTTPS_PORT" "http://127.0.0.1:$MAIL_PORT" >/dev/null
}

serve_off() {
  command -v tailscale >/dev/null 2>&1 || return 0
  tailscale serve --https="$HTTPS_PORT" off >/dev/null 2>&1 || true
  tailscale serve --https="$MAIL_HTTPS_PORT" off >/dev/null 2>&1 || true
}

urls() {
  local host
  host=$(tailnet_host)
  if [ -n "$host" ]; then
    echo "App:     https://$host:$HTTPS_PORT   (tailnet only)"
    echo "Mailpit: https://$host:$MAIL_HTTPS_PORT   (sign-in and verification emails)"
  fi
  echo "Local:   http://127.0.0.1:$APP_PORT   (Mailpit: http://127.0.0.1:$MAIL_PORT)"
}

up() {
  command -v podman >/dev/null 2>&1 || die "podman is required"
  ensure_env
  local host base version
  host=$(tailnet_host)
  if [ -n "$host" ]; then base="https://$host:$HTTPS_PORT"; else base="http://localhost:$APP_PORT"; fi
  # A new version on every build busts the server's JS and service-worker caches.
  version="preview-$(git -C "$ROOT" rev-parse --short HEAD)"
  git -C "$ROOT" diff --quiet HEAD -- 2>/dev/null || version="$version-dirty"
  version="$version-$(date +%m%d%H%M%S)"

  echo "Building $IMAGE ($version) from $ROOT"
  podman build -q --build-arg BUILD_VERSION="$version" -t "$IMAGE" -f "$ROOT/Containerfile" "$ROOT" >/dev/null

  podman volume exists "$VOLUME" || podman volume create "$VOLUME" >/dev/null
  podman pod rm -f "$NAME" >/dev/null 2>&1 || true
  podman pod create --name "$NAME" --label app="$NAME" \
    -p "127.0.0.1:$APP_PORT:8080" -p "127.0.0.1:$MAIL_PORT:8025" >/dev/null
  podman run -d --pod "$NAME" --name "$NAME-postgres" \
    -e POSTGRES_DB=nabu -e POSTGRES_USER=nabu -e POSTGRES_PASSWORD=nabu \
    -v "$VOLUME:/var/lib/postgresql/data" docker.io/library/postgres:17-alpine >/dev/null
  podman run -d --pod "$NAME" --name "$NAME-mailpit" docker.io/axllent/mailpit:v1.32.0 >/dev/null

  # Check TCP: during first-run initdb, PostgreSQL listens on its socket only.
  local i
  for i in $(seq 1 60); do
    podman exec "$NAME-postgres" pg_isready -h 127.0.0.1 -U nabu -d nabu >/dev/null 2>&1 && break
    sleep 1
  done

  # SERVER_SECURE stays false: it would send HSTS for the whole tailnet
  # hostname, which other services on this Mac may serve over plain HTTP.
  # Tailscale Serve already provides HTTPS to the phone.
  podman run -d --pod "$NAME" --name "$NAME-app" --restart=on-failure \
    --env-file "$ENV_FILE" \
    -e PORT=8080 -e APP_BASE_URL="$base" -e SERVER_SECURE=false \
    -e DATABASE_URL="postgres://nabu:nabu@localhost:5432/nabu?sslmode=disable" \
    -e SMTP_HOST=localhost -e SMTP_PORT=1025 -e SMTP_FROM=no-reply@nabu.local \
    -e RATE_LIMIT_AUTH_MAX=1000 -e RATE_LIMIT_JOIN_MAX=1000 \
    "$IMAGE" >/dev/null

  for i in $(seq 1 60); do
    curl -fsS -o /dev/null "http://127.0.0.1:$APP_PORT/ready" 2>/dev/null && break
    [ "$i" = 60 ] && { podman logs --tail 30 "$NAME-app" >&2; die "the app did not become ready"; }
    sleep 1
  done

  [ -n "$host" ] && serve_on
  echo "Preview is up ($version)."
  urls
}

down() {
  serve_off
  podman pod rm -f "$NAME" >/dev/null 2>&1 || true
  echo "Preview stopped. Its database is kept in the $VOLUME volume."
}

reset() {
  [ "${1:-}" = "--yes" ] || die "reset deletes the preview database; run: scripts/preview.sh reset --yes"
  down
  podman volume rm -f "$VOLUME" >/dev/null 2>&1 || true
  echo "Preview database deleted."
}

status() {
  podman ps -a --filter "pod=$NAME" --format '{{.Names}}\t{{.Status}}' 2>/dev/null || true
  urls
}

case "${1:-up}" in
  up) up ;;
  down) down ;;
  reset) reset "${2:-}" ;;
  status) status ;;
  logs) podman logs -f "$NAME-app" ;;
  *) die "usage: scripts/preview.sh [up|down|status|logs|reset --yes]" ;;
esac
