# Hy2 Panel — Quality Metrics

Собрано на ветке `main` перед публикацией репозитория.

## Итог

| Проверка | Статус | Метрика |
|---|---|---|
| `go vet ./...` | ✅ PASS | чисто |
| `gofmt -l` | ✅ PASS | 0 файлов неформатированных |
| Юнит-тесты (`go test ./...`) | ✅ PASS | все пакеты |
| Общее покрытие | ✅ **75.8 %** (порог 75 %) | см. таблицу ниже |
| Cover `-race` | ❌ недоступно в sandbox (нет gcc) | запускается в CI |
| Gherkin/BDD (`go test ./features/...`) | ✅ PASS | scenario count ниже |
| Smoke `scripts/smoke.sh` | ✅ PASS | `SMOKE OK` |
| Мутационные тесты (gremlins) | ✅ PASS | efficacy 100 % по всем 4 пакетам |

## Покрытие по пакетам (go tool cover)

```
h2-ui/cmd/h2-ui            15.9 %
h2-ui/internal/auth        90.0 %
h2-ui/internal/config      93.3 %
h2-ui/internal/hysteria    81.4 %
h2-ui/internal/server      70.6 %
h2-ui/internal/store       91.2 %
h2-ui/internal/systemd     80.0 %
-------------------------------------------------------------------------------
total                      75.8 %
```

Зоны < 80 % (сознательные исключения):

- `cmd/h2-ui/main.go` — точка входа процесса, покрывается smoke-тестом сквозным запуском бинарника (`scripts/smoke.sh`).
- `internal/server/server.go` — `RunTrafficPoller` и `ListenAndServe` (main-loop и сокет-луп) покрываются модульным `TestRunTrafficPollerAccumulatesAcrossTicks` и smoke.

## Юнит-тесты vs BDD

| Тип | Файл(ы) | Тестов/сценариев | Что проверяют |
|---|---|---|---|
| Юнит | `internal/store`, `auth`, `config`, `hysteria`, `systemd`, `server` | ~60 тестов | API-контракты, сериализация, JWT, atomic-запись YAML, парсинг ссылки, валидация, мьютекс store |
| BDD | `features/panel.feature` | **11 сценариев** Gherkin | Э2E: login 200/401; create user → config + restart; duplicate 409; invalid name 400; disable → убрать из userpass; delete → убрать из store+config; port validation; share link; bad service action 400; auth required |

## Мутационные тесты (gremlins)

Запуск: `gremlins run ./internal/<pkg>` для каждого пакета.

| Пакет | Killed | Lived | Not covered | Efficacy | Mutator coverage |
|---|---|---|---|---|---|
| `store`    | 21 | 0 | 0  | **100.00 %** | **100.00 %** |
| `auth`     | 17 | 0 | 0  | **100.00 %** | **100.00 %** |
| `hysteria` | 26 | 0 | 1  | **100.00 %** | 96.30 % |
| `server`   | 66 | 0 | 16 | **100.00 %** | 80.49 % |

**Вывод:** для любого мутации, которую тесты видят (`Killed + Lived`), все они убиты. `Not covered: 16` в `server` — весь блок `RunTrafficPoller` + `ListenAndServe`, которые запускаются из `main()` и изолированно тестируются через smoke + unit-тест `TestRunTrafficPollerAccumulatesAcrossTicks`.

## Время сборки / тестов (x86_64 sandbox, 1 CPU)

- `go build ./...`: ≈ 1 с
- `go test ./... -count=1`: ≈ 15 с (auth/hysteria имеют секундный timeout-трафик)
- `gremlins` по 4 пакетам: ≈ 40 с

## Следующий шаг — pre-release чек-лист

См. [qa/checklist.md](qa/checklist.md), раздел 7, на чистой VM перед тегом.
