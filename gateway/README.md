# Gateway

HTTP API для Ledger. Бюджеты и расходы сохраняются в PostgreSQL,
отчёты кешируются в Redis на 30 секунд. Ledger подключён как библиотека
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
| GET | `/api/reports/summary?from=2026-10-01&to=2026-10-31` | 200, отчёт за период |
| GET | `/ping` | 200, `{"message":"pong"}` |

Суммы — положительные целые числа в копейках. Дата расхода необязательна:
по умолчанию используется сегодня в UTC. Формат даты — `YYYY-MM-DD`.
Пустые списки возвращаются как `[]`.

Ошибки возвращаются в формате `{"error":"описание"}`: 400 — неверные данные,
409 — превышен бюджет, 500 — ошибка БД, 404 — неизвестный путь,
405 — неподдерживаемый метод. JSON-контракты бюджетов и транзакций сохранены.
Маршрут отчёта добавлен в этом задании.

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
curl -i http://localhost:8080/api/budgets

curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
curl -i 'http://localhost:8080/api/reports/summary?from=2026-10-01&to=2026-10-31'
```

Два запроса отчёта подряд дают `cache miss`, затем `cache hit` в логах Ledger.
Если ключ уже есть, для проверки miss дождитесь истечения TTL 30 секунд.

Из корня проекта можно посмотреть строки и TTL напрямую:

```sh
docker compose exec postgres psql -U postgres -d cashapp -c \
  "SELECT * FROM budgets WHERE category='hw7-food'; SELECT * FROM expenses WHERE category='hw7-food';"
docker compose exec redis redis-cli -n 0 TTL report:summary:2026-10-01:2026-10-31
```

Остановите сервер через Ctrl+C, запустите снова и повторите GET списков.
Бюджет и расходы должны сохраниться.

Команды тестов — в [корневом README](../README.md). Из корня также можно
запустить `python3 scripts/smoke.py`: он проверяет HTTP, строки PostgreSQL,
кеш Redis и перезапуск с настройками Compose по умолчанию. Скрипт оставляет
в БД проверочные записи в уникальной категории и выводит её имя.
