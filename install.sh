#!/usr/bin/env bash
# h2-ui one-liner installer:
#   bash <(curl -fsSL https://raw.githubusercontent.com/suffocater-rgb/h2-ui/main/install.sh)
#
# Installs Hysteria 2 (official), downloads the latest h2-ui release,
# creates both systemd units and prints the panel URL + admin credentials.
set -euo pipefail

REPO="${H2UI_REPO:-suffocater-rgb/h2-ui}"
PANEL_PORT="${H2UI_PORT:-2053}"
HY_PORT="${H2UI_HYSTERIA_PORT:-443}"
INSTALL_DIR=/etc/h2-ui
HY_DIR=/etc/hysteria
BIN_DIR=/usr/local/bin

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
die()   { red "error: $*"; exit 1; }

[ "$(id -u)" -eq 0 ] || die "run as root"
command -v curl >/dev/null || die "curl is required (apt/yum install curl)"
command -v systemctl >/dev/null || die "systemd is required"

arch=$(uname -m)
case "$arch" in
  x86_64)  GOARCH=amd64 ;;
  aarch64) GOARCH=arm64 ;;
  *) die "unsupported arch: $arch" ;;
esac

echo "==> installing Hysteria 2"
bash <(curl -fsSL https://get.hy2.sh/) >/dev/null

echo "==> downloading h2-ui ($GOARCH)"
ver=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
[ -n "$ver" ] || die "no release found for ${REPO} — publish one first"
curl -fsSL "https://github.com/${REPO}/releases/download/${ver}/h2-ui-linux-${GOARCH}.tar.gz" -o /tmp/h2-ui.tgz
tar -C /tmp -xzf /tmp/h2-ui.tgz
install -m 0755 /tmp/h2-ui "${BIN_DIR}/h2-ui"

mkdir -p "$INSTALL_DIR" "$HY_DIR"
chmod 700 "$INSTALL_DIR"

# --- initial hysteria config (self-signed, works out of the box) --------
OBFS_PW=$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 16)
FIRST_USER="user1"
FIRST_PW=$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 12)
TRAFFIC_SECRET=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')

if [ ! -f "$HY_DIR/config.yaml" ]; then
cat > "$HY_DIR/config.yaml" <<EOF
listen: :${HY_PORT}

tls:
  type: tls
  cert: ${HY_DIR}/cert.crt
  key: ${HY_DIR}/cert.key

obfs:
  type: salamander
  salamander:
    password: ${OBFS_PW}

masquerade: https://www.bing.com

auth:
  type: userpass
  userpass:
    ${FIRST_USER}: ${FIRST_PW}

trafficStats:
  listen: 127.0.0.1:9999
  secret: ${TRAFFIC_SECRET}
EOF

  # self-signed cert so the server starts with zero preconditions
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout "$HY_DIR/cert.key" -out "$HY_DIR/cert.crt" \
    -subj "/CN=bing.com" >/dev/null 2>&1
fi

echo "==> systemd units"
cat > /etc/systemd/system/h2-ui.service <<EOF
[Unit]
Description=h2-ui panel
After=network.target hysteria-server.service

[Service]
Type=simple
Environment=H2UI_ADDR=0.0.0.0:${PANEL_PORT}
Environment=H2UI_DATA=${INSTALL_DIR}
Environment=H2UI_HYCONF=${HY_DIR}/config.yaml
Environment=H2UI_TRAFFIC_API=http://127.0.0.1:9999
Environment=H2UI_TRAFFIC_SECRET=${TRAFFIC_SECRET}
ExecStart=${BIN_DIR}/h2-ui
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now hysteria-server >/dev/null 2>&1 || systemctl restart hysteria-server
systemctl enable --now h2-ui

# first-run credentials: the binary prints them when the store is fresh;
# we extract them from the journal so the user gets everything on screen.
sleep 2
CREDS=$(journalctl -u h2-ui -n 20 --no-pager 2>/dev/null | sed -n '/h2-ui admin credentials/,/password/p' || true)

ip=$(curl -fsSL --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

green "============================================================"
green  " h2-ui installed"
green  " panel:     http://${ip}:${PANEL_PORT}"
[ -n "$CREDS" ] && green "$CREDS"
green  ""
green  " first hysteria user: ${FIRST_USER} / ${FIRST_PW}"
green  " hysteria port: ${HY_PORT} (obfs: salamander)"
green  "============================================================"
