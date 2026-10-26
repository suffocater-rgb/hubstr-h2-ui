# QA процедуры Hy2 Panel

Этот документ — обязательный контур качества перед любым релизом `v*`.

## 1. Статические проверки

```bash
go vet ./...
test -z "$(gofmt -l cmd internal features web)"
```

Оба должны завершиться без вывода (exit 0).

## 2. Юнит-тесты и покрытие

```bash
go test ./... -count=1 -timeout 120s -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out | tail -1
```

**Пороги:**

- Общее покрытие ≥ **75 %**
- Все ключевые пакеты ≥ **80 %**: `internal/store`, `internal/auth`, `internal/config`
- Новые файлы в `internal/server` ≤ 10 строк без тестов

## 3. BDD (Gherkin / godog)

```bash
go test ./features/... -count=1 -timeout 60s
```

Все сценарии из `features/panel.feature` должны пройти.

## 4. Smoke на собранном бинарнике

```bash
go build -trimpath -ldflags "-s -w" -o ./h2-ui ./cmd/h2-ui
bash scripts/smoke.sh ./h2-ui
```

Ожидаемый вывод: `SMOKE OK`. Проверяет: старт, login, create user, sync config, share link, service restart-валидацию, 401 без токена.

## 5. Мутационные тесты

Запуск по каждому пакету отдельно:

```bash
for pkg in store auth hysteria server; do
  gremlins run ./internal/$pkg
done
```

**Пороги:**

- Test efficacy ≥ **95 %** для каждого пакета
- Not covered мутаций в `internal/server` вне `RunTrafficPoller`/`ListenAndServe` = 0
- Любой «Lived» мутант — повод для разбора в PR

## 6. Метрики качества

После пунктов 2–5 обновляется [TESTING.md](../TESTING.md) со свежими цифрами.

## 7. Интеграционный pre-release чек-лист (ручной)

Перед тегом `vX.Y.Z` на чистой VM Ubuntu 22.04/24.04 или Debian 12 с root:

- [ ] `bash <(curl -fsSL https://raw.githubusercontent.com/<owner>/h2-ui/main/install.sh)` — без ошибок, выводит URL панели и credentials
- [ ] Панель открывается по `http://<ip>:2053` и пускает с выданных credentials
- [ ] Создание/выключение/удаление пользователя меняет `/etc/hysteria/config.yaml`, `systemctl restart hysteria-server` срабатывает
- [ ] `h2://` ссылка из UI открывается NekoBox / Streisand / Hiddify, трафик идёт
- [ ] Раздел «Сервер»: смена порта + obfs + masquerade применяется и перезапускает сервис
- [ ] Раздел «Логи» показывает последние строки journalctl hysteria-server
- [ ] `systemctl status hysteria-server h2-ui` — оба `active (running)`
- [ ] Установка поверх существующего окружения — idempotent (не теряет users/config)

## 8. Релизный чек-лист CI

- [ ] Workflow `ci.yml` на commit в main — зелёный
- [ ] Тег `v*.*.*` запускает job `release` и аттачит `*.tar.gz` к GitHub Release
- [ ] В Assets релиза присутствуют: `h2-ui-linux-amd64.tar.gz`, `h2-ui-linux-arm64.tar.gz`
- [ ] `install.sh` из главной ветки совпадает с содержимым `install.sh` в tarball
