# Gateway

HTTP API для Ledger. Бюджеты и расходы сохраняются в PostgreSQL,
отчёты и список бюджетов кешируются в Redis на 30 секунд. Ledger подключён как библиотека
и запускается в одном процессе с Gateway.

## Запуск

Сначала подготовьте PostgreSQL, Redis и Goose по [инструкции Ledger](../ledger/README.md).
Из корня проекта:

```sh
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export REDIS_DB=0
export REDIS_PASSWORD=''
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
cd gateway
go run .
```

Сервер слушает `:8080`; другой адрес задаётся через `GATEWAY_ADDR`.
При ошибке подключения к PostgreSQL или Redis сервер не запускается.
Для отката последней миграции из корня проекта:

```sh
goose -dir ./ledger/migrations postgres "$DATABASE_URL" down
```

Откат удаляет таблицу расходов. Перед следующим запуском примените `up`.

## Эндпоинты

| Метод | Путь | Ответ |
| --- | --- | --- |
| POST | `/api/budgets` | 201, создать или обновить бюджет |
| GET | `/api/budgets` | 200, массив бюджетов |
| POST | `/api/transactions` | 201, расход с назначенным ID |
| GET | `/api/transactions` | 200, массив расходов |
| POST | `/api/transactions/bulk` | 200, сводка пакетного импорта |
| GET | `/api/reports/summary?from=2026-10-01&to=2026-10-31` | 200, отчёт за период |
| GET | `/ping` | 200, `{"message":"pong"}` |

Суммы — положительные целые числа в копейках. Дата расхода необязательна:
по умолчанию используется сегодня в UTC. Формат даты — `YYYY-MM-DD`.
Пустые списки возвращаются как `[]`.

Ошибки возвращаются в формате `{"error":"описание"}`: 400 — неверные данные,
409 — превышен бюджет, 500 — ошибка БД, 404 — неизвестный путь,
405 — неподдерживаемый метод. JSON-контракты бюджетов и транзакций сохранены.
Маршрут отчёта сохранён из задания 7.

## Пакетный импорт

`POST /api/transactions/bulk` принимает JSON-массив в том же формате, что и
одиночный `POST /api/transactions`. `?workers=4` задаёт размер пула Ledger.
Значение по умолчанию — 4. Число одновременно работающих воркеров ограничено
32 и размером массива. Для `workers` принимается положительное целое число.

Каждая транзакция проверяется и сохраняется отдельно с учётом лимита бюджета.
Завершившийся импорт возвращает `200 OK` со счётчиками `accepted` и `rejected`
и массивом `errors`. Индексы ошибок соответствуют позициям во входном массиве,
начиная с нуля. Пустой массив даёт нулевые счётчики и `errors: []`.

Пример для новой категории `bulk-demo-food`.

```sh
curl -i -X POST http://localhost:8080/api/budgets \
  -H 'Content-Type: application/json' \
  -d '{"category":"bulk-demo-food","limit":500}'

curl -i -X POST 'http://localhost:8080/api/transactions/bulk?workers=3' \
  -H 'Content-Type: application/json' \
  -d '[{"amount":450,"category":"bulk-demo-food","description":"ланч"},
       {"amount":0,"category":"bulk-demo-food"},
       {"amount":600,"category":"bulk-demo-food"}]'
```

```json
{
  "accepted": 1,
  "rejected": 2,
  "errors": [
    {"index": 1, "error": "transaction amount must be positive"},
    {"index": 2, "error": "budget exceeded"}
  ]
}
```

При неверном JSON или значении `workers` Gateway отвечает `400 Bad Request`.
При отмене Ledger прекращает выдачу работ, дожидается воркеров и возвращает
в Gateway частичную сводку и ошибку контекста. HTTP-ответ содержит `504` и
`{"error":"request timeout or canceled"}`. Уже сохранённые расходы остаются в базе.

## Отчёт по категориям

`GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` возвращает суммы расходов
по категориям за период включительно. Суммы указаны в копейках, как во всём API.

Ledger получает список категорий запросом `SELECT DISTINCT category` за период.
Затем для каждой категории запускается отдельная горутина с `SUM(amount)`.
Завершение координирует `sync.WaitGroup`. Каждая горутина пишет только в свою
ячейку среза, а итог собирается после `wg.Wait()`, поэтому гонок нет.
Пока идёт расчёт, фоновая горутина с `time.Ticker` раз в 400 мс пишет в лог
`report: calculating...`. После расчёта или по `ctx.Done()` тикер останавливается.
Все SQL-запросы выполняются через `QueryContext`/`QueryRowContext` с `ctx` запроса.

Пример:

```bash
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{"from":"2026-10-01","to":"2026-10-31","total":252356,
 "categories":[{"category":"еда","amount":123456},
               {"category":"кафе","amount":50000},
               {"category":"транспорт","amount":78900}]}
```

Ошибки:

| Код | Когда | Тело |
|-----|-------|------|
| 400 | нет `from`/`to`, неверный формат даты или `from > to` | `{"error":"from and to must be YYYY-MM-DD with from <= to"}` |
| 504 | истёк таймаут запроса или клиент отменил запрос | `{"error":"request timeout or canceled"}` |
| 500 | ошибка БД | `{"error":"internal server error"}` |

## Таймауты запросов

Middleware `Timeout` применяется ко всем ручкам. Для каждого запроса он создаёт
`context.WithTimeout` и передаёт контекст дальше через `r.WithContext(ctx)`.

- Значение по умолчанию: **2 секунды** (константа `DefaultRequestTimeout` в `internal/api/middleware.go`).
- Значение меняется переменной окружения `REQUEST_TIMEOUT` в формате `time.ParseDuration`:
  `REQUEST_TIMEOUT=500ms`, `REQUEST_TIMEOUT=5s`. При некорректном значении используется 2s.
- При превышении таймаута или отмене запроса API возвращает **504 Gateway Timeout**
  и `{"error":"request timeout or canceled"}`.

Проверка таймаута без зависимости от скорости SQL и попадания в кеш:

```bash
# Из каталога gateway: тесты используют задержанные зависимости и настоящий HTTP.
go test -race ./internal/api -run 'TestTimeoutMiddlewareReturns504|TestReviewDeadlineSuccessReturns504|TestSlowBodyDeadline|TestBodyKeepAlive' -v
```

При ручной проверке `REQUEST_TIMEOUT=1ms go run .` ответ 504 не гарантирован:
быстрый запрос может успеть завершиться. Для проверки SQL-отмены используйте
отдельную тестовую БД и удерживайте блокировку таблицы expenses в транзакции,
пока выполняется запрос отчёта с незакешированным диапазоном. В логах будут
остановка heartbeat и ошибка запроса; точные сообщения зависят от стадии отмены.

Отчёт кешируется на 30 секунд. Для пересчёта выберите новый диапазон или удалите
только нужный ключ: `redis-cli -n 0 DEL report:summary:2026-10-01:2026-10-31`
(адрес и номер Redis DB должны соответствовать настройкам приложения).

## Проверка через cURL

Для повторной проверки используйте новую категорию:

```sh
# 201; повторный запрос обновляет бюджет этой категории.
curl -i -X POST http://localhost:8080/api/budgets \
  -H 'Content-Type: application/json' \
  -d '{"category":"hw7-food","limit":500}'

# 201; в expenses будет amount=4.50.
curl -i -X POST http://localhost:8080/api/transactions \
  -H 'Content-Type: application/json' \
  -d '{"amount":450,"category":"hw7-food","description":"ланч","date":"2026-10-27"}'

# 409: сумма расходов превысила бы 500. Запись не добавляется.
curl -i -X POST http://localhost:8080/api/transactions \
  -H 'Content-Type: application/json' \
  -d '{"amount":100,"category":"hw7-food","date":"2026-10-27"}'

curl -i http://localhost:8080/api/transactions

# Два списка подряд: budgets cache miss, затем budgets cache hit.
curl -i http://localhost:8080/api/budgets
curl -i http://localhost:8080/api/budgets

# Два отчёта подряд: report cache miss, затем report cache hit.
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

В логах Ledger видны `cache miss` и `cache hit` (пример — в
[README Ledger](../ledger/README.md#кеш-redis)). Если ключ уже есть, для
проверки miss дождитесь истечения TTL 30 секунд или удалите его через
`redis-cli DEL`. Любой `POST /api/budgets` удаляет `budgets:all`, поэтому
следующий `GET /api/budgets` снова даёт `cache miss` и сразу показывает
новый лимит. Ответы с кешем и без него одинаковы.

Из корня проекта можно посмотреть строки и TTL напрямую:

```sh
docker compose exec postgres psql -U postgres -d cashapp -c \
  "SELECT * FROM budgets WHERE category='hw7-food'; SELECT * FROM expenses WHERE category='hw7-food';"
docker compose exec redis redis-cli -n 0 TTL report:summary:2026-10-01:2026-10-31
docker compose exec redis redis-cli -n 0 TTL budgets:all
```

Остановите сервер через Ctrl+C, запустите снова и повторите GET списков.
Бюджет и расходы должны сохраниться.

Команды тестов — в [корневом README](../README.md). Из корня также можно
запустить `python3 scripts/smoke.py`: он проверяет HTTP, строки PostgreSQL,
кеш Redis и перезапуск с настройками Compose по умолчанию. Скрипт оставляет
в БД проверочные записи в уникальной категории и выводит её имя.

## Подключение Ledger

В `main` фабрика `ledger.Open(ctx)` возвращает интерфейс `ledger.Service`,
`closeFn` и ошибку. После успешного старта вызывается `defer closeFn()`,
сервис передаётся в `api.NewHandler(service)`. При ошибке инициализации
Gateway пишет её в лог и завершает работу.

HTTP-слой знает только интерфейс сервиса и DTO. Настройки БД, подключения,
репозитории и проверка лимитов находятся в Ledger.

Таймаут охватывает чтение тела запроса и запись кэша. Отмена контекста
прерывает сетевое чтение; JSON-ответ буферизуется до завершения обработчика,
поэтому после дедлайна успешный статус заменяется на 504. Потоковая выдача
ответов этим middleware не поддерживается.
