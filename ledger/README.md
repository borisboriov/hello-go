# Ledger

Бюджеты и расходы хранятся в PostgreSQL. Отчёты за период и список бюджетов
кешируются в Redis на 30 секунд. Требуются Go 1.23+, Docker Compose и Goose.

## Подготовка БД

Команды выполняются из корня проекта:

```sh
docker compose up -d --wait
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export REDIS_DB=0
export REDIS_PASSWORD=''

go install github.com/pressly/goose/v3/cmd/goose@v3.24.3
export PATH="$(go env GOPATH)/bin:$PATH"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
```

Compose создаёт БД `cashapp` и пользователя `postgres` с паролем `postgres`.
Данные PostgreSQL сохраняются в Docker-томе. При локальной установке БД
можно создать командой `createdb -h localhost -U postgres cashapp`.

В каждом файле миграции есть секции `-- +goose Up` и `-- +goose Down`.
Откат последней миграции удаляет таблицу расходов вместе с данными:

```sh
goose -dir ./ledger/migrations postgres "$DATABASE_URL" down
# Применить снова:
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
```

Миграции нужно применять перед запуском приложения и после изменений схемы.
`docker compose down` сохраняет том, а `docker compose down -v` удаляет его.

## Настройки и запуск

`DATABASE_URL` имеет приоритет. Если она не задана, строка подключения
собирается из `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`.
Значения по умолчанию: `localhost:5432`, `postgres/postgres`, `cashapp`,
`sslmode=disable`. Redis: `REDIS_ADDR=localhost:6379`, `REDIS_DB=0`,
`REDIS_PASSWORD` пустой. `REDIS_DB` должен быть неотрицательным целым числом.

Ledger работает внутри Gateway. После миграций с теми же переменными:

```sh
cd gateway
go run .
```

При старте выполняется Ping PostgreSQL и Redis. Успешные подключения
выводятся в лог вместе с прочитанными `REDIS_ADDR` и `REDIS_DB` (пароль не
логируется); при ошибке приложение завершается до запуска HTTP.
Пул PostgreSQL: 10 открытых соединений, 5 простаивающих, lifetime 30 минут.

## Хранение и отчёты

`SetBudget` обновляет бюджет через UPSERT по категории. При добавлении расхода
вызывается `Validate()`, затем проверяется сумма всех расходов категории.
Сервис проверяет лимит через доменные интерфейсы репозиториев.
Проверка и INSERT выполняются в SQL-транзакции. `SELECT ... FOR UPDATE`
блокирует строку бюджета, чтобы параллельные запросы не превысили лимит.
Для категории без бюджета расходы разрешены.

В API суммы указаны целыми копейками, в БД — рублями `NUMERIC(14,2)`:
`amount: 450` соответствует `4.50`. Преобразование выполняется без `float64`.
Пустая дата расхода означает сегодня в UTC, непустая — `YYYY-MM-DD`.
Расходы выводятся по `date DESC, id DESC`, бюджеты — по категории.

`GetReportSummary(ctx, from, to)` считает суммы по категориям за интервал,
включая обе даты. Ключ Redis — `report:summary:<from>:<to>`, значение — JSON,
TTL — 30 секунд. После новых расходов отчёт может отставать до 30 секунд.

## Кеш Redis

Оба кеша работают по схеме cache-aside внутри сервиса, Gateway о них не знает.

| Ключ | Метод | TTL | Когда сбрасывается |
| --- | --- | --- | --- |
| `report:summary:<from>:<to>` | `GetReportSummary` | 30 с | только по TTL |
| `budgets:all` | `ListBudgets` | 30 с | `DEL` после успешного `SetBudget` и по TTL |

Чтение: ключ найден — значение разбирается и возвращается (`cache hit`);
не найден — данные читаются из PostgreSQL, записываются в Redis с TTL и
возвращаются (`cache miss`). Значения хранятся строкой JSON. Если Redis
недоступен или значение повреждено, метод читает PostgreSQL и пишет
`cache read failed` / `cache write failed`, запрос не падает.

`SetBudget` удаляет `budgets:all` только после успешной записи в БД, поэтому
невалидный бюджет или ошибка БД кеш не трогают. Если сам `DEL` не удался,
запись остаётся в силе, а устаревший список живёт не дольше TTL. Проверка
лимита при добавлении расхода читает бюджет напрямую из PostgreSQL
(`SELECT ... FOR UPDATE`), а не из кеша. Список расходов не кешируется.

Два вызова подряд: первый даёт `miss`, второй — `hit`. Реальный лог Gateway
после `DEL` обоих ключей, двух `GET /api/budgets`, двух запросов отчёта,
`POST /api/budgets` и ещё одного `GET /api/budgets`:

```text
Ledger: connected to PostgreSQL (pool: open=10 idle=5)
Ledger: connected to Redis (addr=localhost:6379 db=0)
Gateway service started on 127.0.0.1:8099
budgets cache miss key=budgets:all
budgets cache hit key=budgets:all
report cache miss key=report:summary:2026-10-01:2026-10-31
report cache hit key=report:summary:2026-10-01:2026-10-31
budgets cache invalidated key=budgets:all
budgets cache miss key=budgets:all
```

TTL и содержимое ключей (`redis-cli` внутри контейнера, из корня проекта):

```sh
docker compose exec redis redis-cli -n 0 TTL budgets:all                           # 30
docker compose exec redis redis-cli -n 0 TTL report:summary:2026-10-01:2026-10-31  # 30
docker compose exec redis redis-cli -n 0 GET report:summary:2026-10-01:2026-10-31
# {"from":"2026-10-01","to":"2026-10-31","total":450,"categories":[{"category":"...","amount":450}]}
```

Без Gateway то же показывает `go run ./cmd/demo` из каталога `ledger`: он
дважды вызывает `ListBudgets` и `GetReportSummary` и печатает время вызовов
(`miss` порядка миллисекунд, `hit` — сотни микросекунд). Демо добавляет
расходы в БД, поэтому для чистого запуска задайте отдельные `DATABASE_URL`
и `REDIS_DB`.

Примеры HTTP-запросов, включая два вызова отчёта и проверку сохранения после
перезапуска, находятся в [README Gateway](../gateway/README.md).
Тесты запускаются по инструкции в [корневом README](../README.md).

## Слои

Сущности и контракты находятся в `internal/domain`, бизнес-логика — в
`internal/service`, SQL — в `internal/repository/pg`. Redis скрыт за контрактами
`ReportCache` и `BudgetCache`, поэтому сервис не импортирует клиенты хранения.

Публичный пакет `ledger` открывает внешний интерфейс `Service` и фабрику
`Open(ctx) (Service, func() error, error)`. Фабрика собирает зависимости через
конструкторы; при неудачном старте закрывает уже открытые ресурсы.
Возвращённая функция закрытия безопасна для повторного вызова.
Миграции при старте не запускаются.

Полная структура и пример подключения в Gateway — в [корневом README](../README.md).
