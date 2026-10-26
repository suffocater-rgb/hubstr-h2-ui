#!/usr/bin/env bash
# Hy2 Panel — установка одной командой:
#   bash <(curl -fsSL https://raw.githubusercontent.com/suffocater-rgb/hubstr-h2-ui/main/install.sh)
#
# Что делает:
#   1. Ставит официальный Hysteria 2 (get.hy2.sh)
#   2. Перезаписывает его конфиг на userpass/salamander/masquerade из этого скрипта
#      (старый конфиг бэкапится в /etc/hysteria/config.yaml.bak)
#   3. Скачивает последний релиз h2-ui под архитектуру (amd64/arm64)
#   4. Создаёт 2 systemd-юнита (hysteria-server, h2-ui) и стартует их
#   5. Печатает URL панели + логин/пароль админа + первого пользователя Hysteria
set -euo pipefail

REPO="${H2UI_REPO:-suffocater-rgb/hubstr-h2-ui}"
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

echo "==> installing Hysteria 2 (official)"
curl -fsSL https://get.hy2.sh/ | bash >/dev/null

echo "==> downloading h2-ui ($GOARCH)"
ver=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
[ -n "$ver" ] || die "no release found for ${REPO} — publish one first"
curl -fsSL "https://github.com/${REPO}/releases/download/${ver}/h2-ui-linux-${GOARCH}.tar.gz" -o /tmp/h2-ui.tgz
tar -C /tmp -xzf /tmp/h2-ui.tgz
install -m 0755 /tmp/h2-ui "${BIN_DIR}/h2-ui"

mkdir -p "$INSTALL_DIR" "$HY_DIR"
chmod 700 "$INSTALL_DIR"

# Генерируем секреты
OBFS_PW=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
TRAFFIC_SECRET=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
FIRST_USER="user1"
FIRST_PW=$(head -c 9 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 12)

# Всегда перезаписываем дефолтный конфиг get.hy2.sh (он использует acme+password,
# который панель не умеет читать). Бэкап делаем один раз.
if [ -f "$HY_DIR/config.yaml" ] && [ ! -f "$HY_DIR/config.yaml.bak" ]; then
  cp "$HY_DIR/config.yaml" "$HY_DIR/config.yaml.bak"
fi

cat > "$HY_DIR/config.yaml" <<EOF2
listen: :${HY_PORT}

tls:
  type: tls
  cert: ${HY_DIR}/cert.crt
  key: ${HY_DIR}/cert.key

obfs:
  type: salamander
  salamander:
    password: ${OBFS_PW}

masquerade:
  type: proxy
  proxy:
    url: https://www.bing.com
    rewriteHost: true

auth:
  type: userpass
  userpass:
    ${FIRST_USER}: ${FIRST_PW}

trafficStats:
  listen: 127.0.0.1:9999
  secret: ${TRAFFIC_SECRET}
EOF2

# Self-signed TLS cert если нет
if [ ! -f "$HY_DIR/cert.crt" ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout "$HY_DIR/cert.key" -out "$HY_DIR/cert.crt" \
    -subj "/CN=bing.com" >/dev/null 2>&1
fi

# Права: hysteria работает от отдельного юзера и должен читать конфиг+ключи
chmod 640 "$HY_DIR/config.yaml" "$HY_DIR/cert.key" 2>/dev/null || true
chmod 644 "$HY_DIR/cert.crt"
if id hysteria &>/dev/null; then
  chown root:hysteria "$HY_DIR/config.yaml" "$HY_DIR/cert.key" 2>/dev/null || true
fi

echo "==> systemd units"
cat > /etc/systemd/system/h2-ui.service <<EOF2
[Unit]
Description=Hy2 Panel
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
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF2

# Активируем / перезапускаем юниты
systemctl daemon-reload
systemctl enable hysteria-server >/dev/null 2>&1 || true
systemctl enable h2-ui >/dev/null 2>&1 || true
systemctl restart hysteria-server
systemctl restart h2-ui

# Ждём старта и забираем credentials из лога (панель их печатает при первом запуске)
sleep 3
CREDS=$(journalctl -u h2-ui -n 20 --no-pager 2>/dev/null | sed -n '/h2-ui admin credentials/,/password/p' || true)

ip=$(curl -fsSL --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

green "============================================================"
green  " Hy2 Panel installed successfully"
green  ""
green  " panel:       http://${ip}:${PANEL_PORT}"
if [ -n "$CREDS" ]; then
  green "$CREDS"
else
  green  " credentials:  file /etc/h2-ui/store.json (admin already created)"
  green  " либо сброс:    h2-ui -reset-admin && systemctl restart h2-ui"
fi
green  ""
green  " first hysteria user:  ${FIRST_USER} / ${FIRST_PW}"
green  " hysteria port:         ${HY_PORT} (obfs: salamander)"
green  "============================================================"
