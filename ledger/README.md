# Ledger

Бюджеты и расходы хранятся в PostgreSQL. Отчёты за период кешируются
в Redis на 30 секунд. Требуются Go 1.23+, Docker Compose и Goose.

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
выводятся в лог; при ошибке приложение завершается до запуска HTTP.
Пул PostgreSQL: 10 открытых соединений, 5 простаивающих, lifetime 30 минут.

## Хранение и отчёты

`SetBudget` обновляет бюджет через UPSERT по категории. При добавлении расхода
вызывается `Validate()`, затем проверяется сумма всех расходов категории.
Проверка и INSERT выполняются в SQL-транзакции. `SELECT ... FOR UPDATE`
блокирует строку бюджета, чтобы параллельные запросы не превысили лимит.
Для категории без бюджета расходы разрешены.

В API суммы указаны целыми копейками, в БД — рублями `NUMERIC(14,2)`:
`amount: 450` соответствует `4.50`. Преобразование выполняется без `float64`.
Пустая дата расхода означает сегодня в UTC, непустая — `YYYY-MM-DD`.
Расходы выводятся по `date DESC, id DESC`, бюджеты — по категории.

`GetReportSummary(ctx, from, to)` считает суммы по категориям за интервал,
включая обе даты. Ключ Redis — `report:summary:<from>:<to>`, значение — JSON.
В логах видны `report cache miss` и `report cache hit`. После новых расходов
отчёт может отставать до 30 секунд. Если кеш недоступен или повреждён,
отчёт читается из PostgreSQL. Списки бюджетов и расходов не кешируются.

Примеры HTTP-запросов, включая два вызова отчёта и проверку сохранения после
перезапуска, находятся в [README Gateway](../gateway/README.md).
Тесты запускаются по инструкции в [корневом README](../README.md).
