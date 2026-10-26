# h2-ui

Веб-панель управления **Hysteria 2** в духе 3x-ui: один статический бинарник,
никаких runtime-зависимостей, установка одной командой.

![Go](https://img.shields.io/badge/go-1.23-blue) ![License](https://img.shields.io/badge/license-MIT-green)

## Возможности

- 👤 Пользователи: создать / выключить / удалить, сброс пароля, кик онлайн-сессий
- 🔗 Готовая ссылка `hysteria2://…` для клиентов (NekoBox, Streisand, Hiddify, v2rayNG)
- 📊 Дашборд: статус сервиса, онлайн, суммарный трафик ↑/↓ (через Hysteria Traffic API)
- ⚙️ Настройки сервера: порт, obfs (salamander), masquerade — с перезапуском сервиса
- 📜 Журнал hysteria-server прямо в панели
- 🔒 Встроенная auth (bcrypt + HS256 JWT), хранилище — один JSON-файл
- 🧪 Юнит-тесты, BDD (Gherkin/godog), race-режим, QA-чеклист

## Установка (одна команда)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/suffocater-rgb/hubstr-h2-ui/main/install.sh)
```

Скрипт:

1. Ставит официальный Hysteria 2 (`get.hy2.sh`),
2. Скачивает последний релиз h2-ui под вашу архитектуру (amd64/arm64),
3. Создаёт самоподписанный сертификат (можно потом заменить на ACME) и стартовый конфиг с obfs,
4. Регистрирует два systemd-юнита (`hysteria-server`, `h2-ui`),
5. Печатает URL панели, логин/пароль администратора и первого пользователя Hysteria.

Панель: `http://IP:2053` (переопределяется переменной `H2UI_PORT`).

## Требования

- Ubuntu 20.04+ / Debian 11+ / CentOS-подобный с systemd
- root
- Открытый UDP-порт для Hysteria (по умолчанию 443)

## Пути и переменные

| Что | Где |
|---|---|
| Бинарник панели | `/usr/local/bin/h2-ui` |
| Данные панели (store.json) | `/etc/h2-ui/` |
| Конфиг Hysteria | `/etc/hysteria/config.yaml` |

| Env | Назначение | Дефолт |
|---|---|---|
| `H2UI_ADDR` | адрес панели | `0.0.0.0:2053` |
| `H2UI_DATA` | каталог store.json | `/etc/h2-ui` |
| `H2UI_HYCONF` | конфиг hysteria | `/etc/hysteria/config.yaml` |
| `H2UI_TRAFFIC_API` | traffic API | `http://127.0.0.1:9999` |
| `H2UI_TRAFFIC_SECRET` | секрет traffic API | — |
| `H2UI_ADMIN_USER` / `H2UI_ADMIN_PASS` | первичный админ | сгенерируется |

Сбросить пароль админа: `h2-ui -reset-admin` и перезапустить сервис.

## Безопасность

- Пароли пользователей Hysteria хранятся открытыми — так требует сам протокол userpass; файл `store.json` доступен только root (0700).
- Пароль админа панели — только bcrypt-хэш.
- Трафик-API Hysteria слушает только `127.0.0.1` и требует секрет.
- Все мутации конфига атомарны (`*.tmp` + rename).

> Для production откройте панель только на нужный IP через firewall или
> повесьте за reverse-proxy с TLS (пример nginx в `docs/reverse-proxy.md`).

## Разработка

```bash
make build        # бинарник в ./h2-ui
make test         # юниты с -race и покрытием
make bdd          # Gherkin-сценарии (godog)
make smoke        # smoke против живого процесса (scripts/smoke.sh)
make coverage     # HTML-отчёт coverage.html
```

## Лицензия

MIT, см. [LICENSE](LICENSE).

## Качество и тесты

Перед публикацией репозитория пройден полный контур:

- ✅ `go vet` + `gofmt` — чисто
- ✅ Юнит-тесты всех пакетов + покрытие **75.8 %** (`internal/store` 91.2 %, `internal/config` 93.3 %)
- ✅ BDD (Gherkin / godog) — 11 сценариев Э2E
- ✅ Smoke-скрипт поверх собранного бинарника (`scripts/smoke.sh`)
- ✅ Мутационные тесты (gremlins): **100 % efficacy** по всем 4 пакетам (`store`, `auth`, `hysteria`, `server`)

См. [TESTING.md](TESTING.md) для подробного отчёта и [qa/checklist.md](qa/checklist.md) для pre-release процедуры.

## GitHub Actions (опционально)

Файл CI лежит в `docs/ci.yml.example`. Чтобы включить автоматические тесты и релизы при пуше тега:

```bash
mkdir -p .github/workflows
cp docs/ci.yml.example .github/workflows/ci.yml
git add .github/workflows/ci.yml
git commit -m "ci: enable GitHub Actions"
git push
```

(нужен токен со scope `workflow`, поэтому не включаем по умолчанию)
