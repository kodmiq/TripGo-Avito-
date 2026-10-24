# Trip Service

HTTP-сервис поездок TripGo. Лабораторная работа 1: создание, получение и завершение поездки, PostgreSQL, миграции goose, менеджер транзакций.

Модуль: `github.com/kodmiq/TripGo-Avito-`.

## Что нужно заранее

- Go 1.26
- `tripgoctl` и запущенный Docker
- В корне лежит `environment.toml` (`lab = 1`, компонент `postgres`)

## Запуск

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect

make migrate
make run
```

`make run` читает `.env.example`, затем перекрывает его файлом `.env`, который создаёт `tripgoctl`. Адреса и пароль базы в код не зашиты. Если сервис пишет, что обязательная переменная не задана, скопируйте её из `.env.example` в `.env`.

Остановить процесс: `Ctrl+C` или `SIGTERM`. Сервис перестаёт принимать запросы, дожидается текущих и закрывает пул. Если не уложился в `SHUTDOWN_TIMEOUT`, в лог пишется ошибка и процесс выходит.

## Переменные окружения

ЛР1 читает эти переменные. Пустое значение или таймаут `0s` — сервис не стартует.

| Переменная | Пример | Зачем |
|---|---|---|
| `HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | `10s` | бюджет остановки |
| `HTTP_READ_TIMEOUT` | `5s` | `ReadTimeout` |
| `HTTP_READ_HEADER_TIMEOUT` | `2s` | `ReadHeaderTimeout` |
| `HTTP_WRITE_TIMEOUT` | `10s` | `WriteTimeout` |
| `HTTP_IDLE_TIMEOUT` | `60s` | `IdleTimeout` |
| `DATABASE_URL` | из `tripgoctl` | строка подключения PostgreSQL |
| `DATABASE_MAX_CONNS` | `10` | размер пула |
| `DATABASE_MIN_CONNS` | `2` | минимум соединений |
| `DATABASE_MAX_CONN_LIFETIME` | `30m` | время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | `5s` | таймаут подключения и `/ready` |
| `DATABASE_QUERY_TIMEOUT` | `3s` | таймаут каждого запроса к базе |

В `.env.example` также лежат имена переменных следующих лабораторных. Сервис их пока не читает, переименовывать нельзя.

## Команды

| Команда | Что делает |
|---|---|
| `make generate` | заново генерирует `internal/generated/api.gen.go` |
| `make migrate` | накатывает миграции |
| `make migrate-down` | откатывает одну последнюю миграцию |
| `make migrate-reset` | откатывает все миграции |
| `make run` | запускает сервис |
| `make test` | `go test -race -count=1 ./...` |

Сгенерированный файл коммитится и руками не правится.

## Ручки

| Метод | Путь | Успех |
|---|---|---|
| `POST` | `/api/v1/trips` | `201` и заголовок `Location` |
| `GET` | `/api/v1/trips/{tripId}` | `200` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200` |
| `GET` | `/health` | `200`, в базу не ходит |
| `GET` | `/ready` | `200`, если база доступна, иначе `503` |

`/health` и `/ready` отвечают `application/json`: `{"status":"ok"}` или `{"status":"unavailable"}`.

Ошибки бизнес-ручек — `application/problem+json`:

| HTTP | `code` | Когда |
|---|---|---|
| 400 | `invalid_request` | плохое тело или `tripId` не UUID |
| 404 | `trip_not_found` | поездки нет |
| 409 | `driver_busy` | у водителя уже есть активная поездка |
| 409 | `trip_completed` | поездка уже завершена |
| 500 | `internal_error` | всё остальное, подробности только в логе |

Заголовок `Idempotency-Key` в этой работе игнорируется. Это задание со звёздочкой.

## Как проверить

Адрес ниже — `:8080`. Если `tripgoctl connect` показывает другой порт, подставьте его.

```bash
make migrate
make run
```

В другом терминале:

```bash
# живой процесс, без базы
curl -sS -D- localhost:8080/health

# база доступна
curl -sS -D- localhost:8080/ready

export USER_ID=5cb72c04-7650-45c9-a79b-bcdba0631e0c
export DRIVER_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')

curl -sS -D- -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' \
  -d "{\"user_id\":\"$USER_ID\",\"driver_id\":\"$DRIVER_ID\",\"start_point\":{\"latitude\":59.9398,\"longitude\":30.3146},\"end_point\":{\"latitude\":59.9290,\"longitude\":30.3626},\"price\":1450}"

# тот же водитель ещё раз — 409 driver_busy
# подставьте id из ответа
curl -sS -D- localhost:8080/api/v1/trips/$TRIP_ID
curl -sS -D- -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish
# повторное завершение — 409 trip_completed
curl -sS -D- -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish

# не UUID — 400 invalid_request
curl -sS -D- localhost:8080/api/v1/trips/not-a-uuid

# нет такой поездки — 404
curl -sS -D- -X POST localhost:8080/api/v1/trips/1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111/finish
```

Откат миграций и повторный накат:

```bash
make migrate-reset
make migrate
```

`/ready` при остановленной базе. Сервис не останавливать:

```bash
tripgoctl environment stop
curl -sS -D- localhost:8080/ready    # 503 {"status":"unavailable"}
curl -sS -D- localhost:8080/health   # 200 {"status":"ok"}
tripgoctl environment start
curl -sS -D- localhost:8080/ready    # снова 200
```

Если после `environment start` порт базы изменился, перезапустите `make run`.

Остановка по сигналу:

```bash
kill -TERM "$(pgrep -x trip-service | head -n1)"
```

В логе будет `shutdown signal received`, затем `shutdown complete`. Процесс должен выйти быстрее, чем `SHUTDOWN_TIMEOUT`.

Двадцать одновременных созданий на одного водителя. Ровно один ответ `201`, остальные `409`:

```bash
DRIVER_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
cat > /tmp/trip.json <<EOF
{"user_id":"$USER_ID","driver_id":"$DRIVER_ID","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.9290,"longitude":30.3626},"price":1450}
EOF

seq 20 | xargs -P20 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' \
  -d @/tmp/trip.json | sort | uniq -c
```

Два одновременных завершения. Один `200`, один `409`, `finished_at` один:

```bash
seq 2 | xargs -P2 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish | sort | uniq -c
```

Атомарность создания. В `internal/trip_service.go` в `Create` временно верните ошибку сразу после вставки в `trips` и до вставки в журнал, соберите сервис и создайте поездку. Ответ будет `500`. В таблице `trips` этой строки не будет: транзакция откатилась. Верните код назад.

Тесты этой работы не требуются. `make test` гоняет `go test` с `-race` и должен завершаться успешно.

## Решения

**Уровень изоляции — Read Committed.** Запрет второй активной поездки держит частичный уникальный индекс `trips_one_active_driver_idx` по `driver_id WHERE status = 'active'`. Он работает в Read Committed: второй `INSERT` получает `23505`, и это `driver_busy`, а не `500`. Repeatable Read или Serializable тут не нужны и добавили бы ошибки сериализации, которые всё равно пришлось бы повторять. Индекс они не заменяют.

**Завершение.** `UPDATE ... WHERE id = $1 AND status = 'active'`. Между проверкой и записью нет окна в коде. Второй параллельный `finish` ждёт блокировку строки, после коммита первого не находит активную строку и получает `409 trip_completed`. `finished_at` не перезаписывается. Если строк ноль, отдельный `SELECT` отличает «нет поездки» от «уже завершена».

**Менеджер транзакций.** `Do` открывает транзакцию и кладёт её в `context`. Репозиторий берёт из контекста транзакцию, а если её нет — пул. В метод репозитория транзакция не передаётся. `nil` из функции — `COMMIT`, ошибка или паника — `ROLLBACK`, паника пробрасывается дальше. Вложенный `Do` не открывает вторую транзакцию и не делает свой `COMMIT`: это делает только внешний вызов. Создание поездки — один `Do` на `INSERT` в `trips` и `INSERT` в `trip_status_history`.

**Зачем `/ready` отдельно от `/health`.** `/health` отвечает, пока жив процесс, и не зависит от базы: оркестратор не должен перезапускать контейнер из-за короткого сбоя PostgreSQL. `/ready` проверяет базу и при отказе отдаёт `503`, чтобы в процесс не шли новые запросы.
