#!/usr/bin/env bash
# Smoke test: boots the real binary against a temp dir (no systemd, no
# hysteria) and walks the critical API path:
#   login -> create user -> read config -> link -> service action -> 401 check
# Exit code 0 = pass.
set -euo pipefail

BIN="${1:-./h2-ui}"
[ -x "$BIN" ] || { echo "build first: make build"; exit 1; }

TMP=$(mktemp -d)
trap 'kill $PID 2>/dev/null || true; rm -rf "$TMP"' EXIT

export H2UI_ADDR=127.0.0.1:18099
export H2UI_DATA="$TMP/data"
export H2UI_HYCONF="$TMP/hy/config.yaml"
export H2UI_ADMIN_USER=smoke
export H2UI_ADMIN_PASS=smokepw
mkdir -p "$TMP/hy"
echo "listen: :443" > "$H2UI_HYCONF"

"$BIN" >"$TMP/log" 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true; rm -rf "$TMP"' EXIT
sleep 1

B=http://127.0.0.1:18099
fail() { echo "SMOKE FAIL: $*"; cat "$TMP/log"; exit 1; }

TOKEN=$(curl -fsS -X POST "$B/api/login" -H 'Content-Type: application/json' \
  -d '{"username":"smoke","password":"smokepw"}' | grep -oP '"token":"\K[^"]+')
[ -n "$TOKEN" ] || fail "no token"

AUTH="Authorization: Bearer $TOKEN"

curl -fsS -o /dev/null -X POST "$B/api/users" -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"username":"ivan","password":"pw1234"}' || fail "create user"

grep -q "ivan" "$H2UI_HYCONF" || fail "config not synced"

curl -fsS "$B/api/users" -H "$AUTH" | grep -q '"ivan"' || fail "list users"

curl -fsS "$B/api/link?user=ivan" -H "$AUTH" | grep -q 'hysteria2://ivan:pw1234@' || fail "link"

code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$B/api/service" -H "$AUTH" \
  -H 'Content-Type: application/json' -d '{"action":"explode"}')
[ "$code" = "400" ] || fail "bad service action: $code"

code=$(curl -s -o /dev/null -w '%{http_code}' "$B/api/users")
[ "$code" = "401" ] || fail "unauthenticated access allowed: $code"

curl -fsS -o /dev/null -X DELETE "$B/api/users/ivan" -H "$AUTH" || fail "delete user"

echo "SMOKE OK"
