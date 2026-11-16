# Trip Service

HTTP-сервис поездок TripGo. Лабораторная работа 3: координаты маршрута, выборка поездок без свежих точек и опрос устройства через ограниченную очередь и пул воркеров.

Модуль: `github.com/kodmiq/TripGo-Avito-`.

## Что нужно заранее

- Go 1.26
- `golangci-lint` v2
- `tripgoctl` и запущенный Docker
- В корне лежит `environment.toml` (`lab = 3`, компоненты `postgres`, `observability` и `push`)

## Запуск

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect

make migrate
make run
```

`make run` читает `.env.example`, затем перекрывает его файлом `.env`, который создаёт `tripgoctl`. Адреса и пароль базы в код не зашиты. Если сервис пишет, что обязательная переменная не задана, скопируйте её из `.env.example` в `.env`.

Остановить процесс: `Ctrl+C` или `SIGTERM`. Сервис перестаёт принимать запросы, дожидается текущих, сбрасывает телеметрию и закрывает пул. Если не уложился в `SHUTDOWN_TIMEOUT`, в лог пишется ошибка и процесс выходит. Недоступный коллектор трейсов старт не останавливает: экспорт идёт в фоне.

## Переменные окружения

Сервис читает эти переменные. Пустое значение, ноль или таймаут `0s` — процесс не стартует.

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
| `IDEMPOTENCY_TTL` | `24h` | сколько хранить `Idempotency-Key` |
| `OTEL_SERVICE_NAME` | `trip-service` | `service.name` в ресурсе телеметрии |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | базовый адрес коллектора, путь сигнала SDK дописывает сам |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | `http/protobuf` или `grpc` |
| `OTEL_RESOURCE_ATTRIBUTES` | `service.namespace=tripgo,deployment.environment=local` | пары `key=value` через запятую |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` | `always_on`, `always_off`, `parentbased_always_on`, `parentbased_always_off` |
| `POSITION_CHECK_INTERVAL` | `30s` | период сканера. Это же пауза между повторными пушами по одной поездке |
| `POSITION_STALE_AFTER` | `60s` | поездка попадает в выборку, если с последней координаты (или с `started_at`) прошло больше этого срока |
| `POSITION_WORKERS` | `4` | фиксированное число воркеров |
| `POSITION_QUEUE_SIZE` | `100` | ёмкость очереди |
| `POSITION_SCAN_BATCH_SIZE` | `100` | сколько строк сканер читает за один запрос |
| `PUSH_HTTP_URL` | из `tripgoctl` | базовый URL Push Service |
| `PUSH_REQUEST_TIMEOUT` | `1s` | таймаут одного вызова Push Service |

`PUSH_ADMIN_URL` сервис не читает: им пользуются вручную, чтобы задержать заглушку. Имена переменных ЛР4 и ЛР5 лежат в `.env.example` заранее, переименовывать их нельзя.

## Команды

| Команда | Что делает |
|---|---|
| `make generate` | заново генерирует `internal/generated/api.gen.go` |
| `make migrate` | накатывает миграции |
| `make migrate-down` | откатывает одну последнюю миграцию |
| `make migrate-reset` | откатывает все миграции |
| `make run` | запускает сервис |
| `make test` | unit-тесты с `-race`; в конце печатает покрытие `domain` и `usecase` |
| `make test-integration` | PostgreSQL через testcontainers, тег `integration` |
| `make lint` | `golangci-lint run ./...` |
| `make docker-build` | собирает образ `trip-service:lab1` |

Сгенерированный файл коммитится и руками не правится.

## Ручки

| Метод | Путь | Успех |
|---|---|---|
| `POST` | `/api/v1/trips` | `201` и заголовок `Location` |
| `GET` | `/api/v1/trips/{tripId}` | `200` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200` |
| `POST` | `/api/v1/trips/{tripId}/positions` | `201` |
| `GET` | `/api/v1/trips/{tripId}/positions` | `200`, все точки по `recorded_at`, затем по `id` |
| `GET` | `/health` | `200`, в базу не ходит |
| `GET` | `/ready` | `200`, если база доступна, иначе `503` |

`/health` и `/ready` отвечают `application/json`: `{"status":"ok"}` или `{"status":"unavailable"}`.

Ошибки бизнес-ручек — `application/problem+json`:

| HTTP | `code` | Когда |
|---|---|---|
| 400 | `invalid_request` | плохое тело, `tripId` не UUID, координаты вне диапазона или `recorded_at` в будущем больше чем на минуту |
| 404 | `trip_not_found` | поездки нет |
| 409 | `driver_busy` | у водителя уже есть активная поездка |
| 409 | `trip_completed` | поездка уже завершена |
| 409 | `idempotency_conflict` | тот же `Idempotency-Key`, но другое тело |
| 500 | `internal_error` | всё остальное, подробности только в логе |

`Idempotency-Key` необязателен. Без него создание работает как раньше и отвечает `201`. Ключ присылает клиент, это UUID. Сервис хранит его `IDEMPOTENCY_TTL` (по умолчанию 24 часа), потом тот же ключ можно использовать для новой поездки.

| Запрос | Ответ |
|---|---|
| первый с этим ключом | `201`, поездка создана |
| повтор с тем же ключом и тем же телом | `200` и та же поездка, вторая не создаётся |
| тот же ключ и другое тело, пока ключ жив | `409 idempotency_conflict` |
| ключ не UUID | `400 invalid_request` |

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

Идемпотентность. Повтор отдаёт `200` и тот же `id`. Другая цена с тем же ключом — `409`.

```bash
KEY=$(cat /proc/sys/kernel/random/uuid)
DRIVER_ID=$(cat /proc/sys/kernel/random/uuid)
BODY='{"user_id":"'"$USER_ID"'","driver_id":"'"$DRIVER_ID"'","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.9290,"longitude":30.3626},"price":1450}'

curl -sS -D- -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' -H "Idempotency-Key: $KEY" -d "$BODY"
curl -sS -D- -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' -H "Idempotency-Key: $KEY" -d "$BODY"
```

Двадцать параллельных запросов с одним новым ключом и одним телом: один `201`, остальные `200`, в базе одна поездка.

```bash
KEY=$(cat /proc/sys/kernel/random/uuid)
DRIVER_ID=$(cat /proc/sys/kernel/random/uuid)
BODY='{"user_id":"'"$USER_ID"'","driver_id":"'"$DRIVER_ID"'","start_point":{"latitude":59.9398,"longitude":30.3146},"end_point":{"latitude":59.9290,"longitude":30.3626},"price":1450}'
seq 20 | xargs -P20 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' -H "Idempotency-Key: $KEY" \
  -d "$BODY" | sort | uniq -c
```

Два одновременных завершения. Один `200`, один `409`, `finished_at` один:

```bash
seq 2 | xargs -P2 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish | sort | uniq -c
```

Атомарность создания. В `insertTrip` временно верните ошибку сразу после вставки в `trips` и до вставки в журнал, соберите сервис и создайте поездку. Ответ будет `500`. В таблицах `trips` и `idempotency_keys` этой поездки не останется: транзакция откатилась. Верните код назад.

Тесты:

```bash
make test                 # -race, покрытие domain и usecase
make test-integration     # нужен Docker
make lint
```

`make test` интеграционные тесты не запускает: у них тег `integration`.

## Архитектура

Зависимости направлены внутрь. Транспорт и репозитории знают про usecase, usecase знает про домен, домен не импортирует `net/http`, `chi`, `pgx`, `squirrel` и OpenTelemetry. Это же правило для домена и usecase проверяет `depguard` в `make lint`.

| Слой | Где | От кого зависит |
|---|---|---|
| вход | `cmd/service` | только `config` и `container`: читает окружение, запускает сервер, по сигналу останавливает HTTP, затем фон |
| сборка | `internal/container` | единственное место, где создаются пул, телеметрия, репозитории, usecase, клиент Push Service и пул воркеров |
| транспорт | `internal/http` | usecase через интерфейс в `contract.go` своей ручки. `repository` и `infra` отсюда не импортируются |
| сценарий | `internal/usecase/trip/{create,get,finish,addposition,listpositions}` | домен и узкие интерфейсы репозитория, транзакции, часов, идентификаторов, метрик |
| фон | `internal/worker/positions` | очередь и пул. Интерфейс Push Service объявлен здесь, реализация — в `internal/gateway/push` |
| правила | `internal/domain/trip` | ни от какого другого пакета сервиса. Переход `active → completed` — метод `Finish` |
| данные | `internal/repository` | SQL и модель. Реализует интерфейсы, которые объявил usecase |
| внешний мир | `internal/infra` | пул pgx, менеджер транзакций, OpenTelemetry и `slog` |

Структуры, которые ходят между слоями, лежат в `internal/model`. Ошибки домена — в `internal/domain/trip`. В HTTP их переводит одна функция, `internal/http/domainerror.Problem`. Текст ошибки базы в ответ не попадает.

Интерфейсы объявляет потребитель, в своём `contract.go`, и они неэкспортируемые: пакету нужно описать, что он требует, а не что умеет чужая реализация. Моки генерирует `mockgen` в `mocks_test.go` того же каталога, пакет тестов — `*_test`. Тест снаружи пакета может передать мок в конструктор, потому что параметр — неэкспортируемый интерфейс, а не именованный тип мока.

Порядок middleware: `Recover`, затем `Tracing`, `Logging`, `Metrics`, затем роутер. В лог запроса попадают шаблон маршрута, статус и длительность. `trace_id` и `span_id` добавляет обработчик логгера, если запись сделана внутри спана. В `http.route` кладётся шаблон chi, например `/api/v1/trips/{tripId}`, а не идентификатор поездки. Счётчики `tripgo.trips.created` и `tripgo.trips.completed` увеличиваются после успешного коммита, не при повторе идемпотентного запроса. `tripgo.trips.active` — наблюдаемая величина, её считает `CountActive` в момент сбора.

От дерева в задании есть три добавки, все по тем же двум правилам — пакет на действие и инфраструктура отдельно от правил:

- `internal/http/health` и `internal/http/ready` — пробы из первой работы, отдельные ручки.
- `internal/repository/idempotency` — хранение ключа идемпотентности, это не сценарий поездки.
- точка входа — `cmd/service`, как в дереве этой работы.

Сгенерированный клиент контракта остаётся в `internal/generated` и исключён из линтера: этот код не пишется руками.

## Опрос устаревших поездок

Пока поездка `active`, устройство шлёт координаты. Если их давно не было, сервис сам просит точку через Push Service. На каждую поездку горутина не заводится: активных поездок планируются десятки тысяч, и таймер на поездку столько горутин не переживёт.

```
тикер
  → сканер (один запрос за порцией, из БД)
    → очередь с приоритетом, ёмкость POSITION_QUEUE_SIZE
      → POSITION_WORKERS воркеров
        → HTTP Push Service, kind = REQUEST_POSITION
```

Сканер — одна горутина с периодом `POSITION_CHECK_INTERVAL`. Пока тик не закончился, следующий не стартует. Выборка идёт из PostgreSQL, в памяти список поездок не хранится. В неё попадают только `active`, у которых `COALESCE(last_position_at, started_at)` старше `POSITION_STALE_AFTER`. Порог вычитается в SQL. Порции по `POSITION_SCAN_BATCH_SIZE` продолжаются ключом `(silent_since, id)`, без `OFFSET`: одинаковое время на границе порции не теряет строку и не читает её дважды. Первый `dropped` на тике останавливает чтение, следующий тик снова начинается с самых старых.

Очередь — не канал. Воркер всегда берёт поездку, которая молчит дольше всех (`last_position_at`, а если координат не было — `started_at`). Одна поездка не стоит в очереди дважды: это map под тем же мьютексом, что и куча. Сканер, воркеры и хендлер координаты берут этот мьютекс. Когда координата сохранилась, хендлер удаляет поездку из очереди, и в Push Service запрос не уходит.

Если очередь полна, новая поездка сравнивается с самой свежей в ней. Молчит дольше — занимает её место, вытесненная считается `dropped`. Свежее всех — дропается сама. Постановка не блокируется: медленный Push Service не останавливает сканер и HTTP. Поездка не пропадает: на следующем тике она снова в выборке, если координата так и не пришла. Период тикера и есть пауза между повторными пушами, отдельная отметка об отправке не хранится.

Воркеров ровно `POSITION_WORKERS`, горутина на задачу не создаётся. Вызов Push Service идёт своим `http.Client` с таймаутом `PUSH_REQUEST_TIMEOUT`, одна попытка. Ошибка транспорта и код `4xx`/`5xx` — разные ошибки; воркер пишет `Warn` и берёт следующую задачу.

Остановка по `SIGINT`/`SIGTERM` в пределах одного `SHUTDOWN_TIMEOUT`:

1. HTTP перестаёт принимать запросы и дожидается текущих.
2. Сканер останавливается, новые задачи не появляются.
3. Воркеры дорабатывают то, что уже в очереди.
4. Закрываются клиент Push Service, телеметрия и пул БД.

Метрики: `tripgo.positions.saved`, `tripgo.stale_checks{result=queued|duplicate|dropped}`, `tripgo.stale_checks.rows_read`, `tripgo.position_requests.queue.size`, `tripgo.position_requests.workers.active`, `tripgo.position_requests{result=success|error|cancelled}`, `tripgo.position_requests.duration`. Спан `position.request` связан со спаном проверки ссылкой, не как ребёнок: обработка переживает тик.

Проверка. Создать поездку и не слать координаты. Через `POSITION_STALE_AFTER` в `tripgoctl environment logs push-service` появляется `REQUEST_POSITION`, не чаще одного раза за `POSITION_CHECK_INTERVAL`. Если слать координаты каждые 5 секунд, команд не будет.

```bash
while true; do
  curl -sS -X POST localhost:8080/api/v1/trips/$TRIP_ID/positions \
    -H 'content-type: application/json' \
    -d "{\"coordinates\":{\"latitude\":59.93,\"longitude\":30.33},\"recorded_at\":\"$(date -u +%FT%TZ)\"}" > /dev/null
  sleep 5
done
```

Задержка заглушки не должна останавливать сканер и ручки. После проверки сбросить её в ноль:

```bash
curl -fsS -X POST "$PUSH_ADMIN_URL/admin/behaviour" \
  -H 'content-type: application/json' -d '{"latency_ms":5000}'
curl -fsS -X POST "$PUSH_ADMIN_URL/admin/behaviour" \
  -H 'content-type: application/json' -d '{"latency_ms":0}'
```

## Решения

**Уровень изоляции — Read Committed.** Запрет второй активной поездки держит частичный уникальный индекс `trips_one_active_driver_idx` по `driver_id WHERE status = 'active'`. Он работает в Read Committed: второй `INSERT` получает `23505`, и это `driver_busy`, а не `500`. Repeatable Read или Serializable тут не нужны и добавили бы ошибки сериализации, которые всё равно пришлось бы повторять. Индекс они не заменяют.

**Завершение.** `UPDATE ... WHERE id = $1 AND status = 'active'`. Между проверкой и записью нет окна в коде. Второй параллельный `finish` ждёт блокировку строки, после коммита первого не находит активную строку и получает `409 trip_completed`. `finished_at` не перезаписывается. Если строк ноль, отдельный `SELECT` отличает «нет поездки» от «уже завершена».

**Менеджер транзакций.** `Do` открывает транзакцию и кладёт её в `context`. Репозиторий берёт из контекста транзакцию, а если её нет — пул. В метод репозитория транзакция не передаётся. `nil` из функции — `COMMIT`, ошибка или паника — `ROLLBACK`, паника пробрасывается дальше. Вложенный `Do` не открывает вторую транзакцию и не делает свой `COMMIT`: это делает только внешний вызов. Создание поездки — один `Do`: при наличии ключа сначала `INSERT` в `idempotency_keys`, затем в `trips` и `trip_status_history`. Внешний ключ на поездку отложен до конца транзакции, поэтому ключ можно записать раньше строки поездки. Если журнал не записался, откатываются и поездка, и ключ.

**Идемпотентность.** Тело сравнивается по полям запроса, а не по сырому JSON: другой порядок полей и пробелы не считаются другим телом. Хеш — SHA-256. От гонки защищает первичный ключ `idempotency_keys`: второй параллельный `INSERT` ждёт коммит первого и получает `23505`. Проигравший откатывает свою поездку и читает уже сохранённую. Истёкшая строка удаляется только если у неё тот же `expires_at`, который увидел запрос, и в той же транзакции, где создаётся новая поездка.

**Зачем `/ready` отдельно от `/health`.** `/health` отвечает, пока жив процесс, и не зависит от базы: оркестратор не должен перезапускать контейнер из-за короткого сбоя PostgreSQL. `/ready` проверяет базу и при отказе отдаёт `503`, чтобы в процесс не шли новые запросы.

## Образ

`deploy/Dockerfile` собирает сервис в Alpine, а в итоговый образ кладёт только статический бинарь. База образа — `gcr.io/distroless/static-debian12:nonroot`, процесс запускается как `nonroot`, не как root. Исходников, Go toolchain и `.env` в итоговом образе нет. `.dockerignore` не пускает в контекст сборки `.git`, `.env`, `.tripgo` и тесты.

Размер итогового образа: **21.3 MB** (`docker images trip-service:lab1`).

Рядом с PostgreSQL из `tripgoctl`. `localhost` внутри контейнера — не хост, поэтому в `DATABASE_URL` нужен `host.docker.internal` и порт из `tripgoctl connect`.

```bash
make docker-build
export DATABASE_URL='postgres://tripgo:tripgo@host.docker.internal:21032/tripgo?sslmode=disable'
docker compose -f deploy/compose.yaml up
curl -sS -D- localhost:8080/ready
```

Порт `21032` замените на тот, что напечатал `tripgoctl connect`.
