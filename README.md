# Система аналитики личных трат

Автор: Соскин Борис Яковлевич.

В ДЗ №11 Gateway запускается в Docker Compose вместе с PostgreSQL и Redis.

Ledger хранит бюджеты и расходы в PostgreSQL. Перед добавлением расхода
проверяется лимит категории. Отчёты и список бюджетов кешируются в Redis
на 30 секунд.
Gateway использует прежние URL и JSON для бюджетов и транзакций.

В проекте три Go-модуля: корневой пример, Ledger и Gateway. Ledger подключён
к Gateway как библиотека, поэтому HTTP-сервер запускается одной командой.

- [Подготовка PostgreSQL, Redis и миграций](ledger/README.md)
- [Запуск Gateway и проверка через cURL](gateway/README.md)
- [ДЗ №10. Пакетный импорт транзакций](gateway/README.md#пакетный-импорт)

## Запуск в Docker Compose

Понадобятся Docker с BuildKit, Docker Compose 2.17+ и Goose. Выполняйте
команды из корня репозитория. Установка Goose описана в [README Ledger](ledger/README.md#подготовка-бд).

Если после предыдущего задания ещё запущен контейнер `hello-go-postgres-1`,
остановите его перед запуском `db`. При прежнем имени Compose-проекта
сервис `db` подключит уже созданный том PostgreSQL.

1. Запустите PostgreSQL. Флаг `--wait` дожидается статуса `healthy`.

   ```sh
   docker compose up -d --wait db
   ```

2. Примените миграции Goose с хоста через опубликованный порт PostgreSQL.

   ```sh
   export DATABASE_URL="postgres://postgres:postgres@127.0.0.1:${POSTGRES_PORT:-5432}/cashapp?sslmode=disable"
   goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
   ```

3. Соберите и запустите Gateway. Compose также запустит Redis и дождётся
   готовности обеих зависимостей. В выводе `ps` статус Gateway — `healthy`.

   ```sh
   docker compose up -d --build --wait gateway
   docker compose ps
   ```

| Переменная | Значение по умолчанию |
| --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | `postgres`, `postgres`, `cashapp` в сервисе `db` |
| `POSTGRES_PORT`, `REDIS_PORT` | Опубликованные порты хоста `5432` и `6379` на `127.0.0.1` |
| `DATABASE_URL` | В Gateway — `postgres://postgres:postgres@db:5432/cashapp?sslmode=disable`. Для Goose на хосте используется адрес из шага 2 |
| `REDIS_ADDR` | В Gateway — `redis:6379` |
| `GATEWAY_ADDR` | `:8080` внутри контейнера. Compose публикует порт `8080` |

### Проверка HTTP

```sh
docker compose exec gateway wget -qO- http://localhost:8080/ping
curl -fsS http://localhost:8080/ping
# В обоих случаях: {"message":"pong"}

base=http://localhost:8080
category="hw11-$(date +%s)-$$"
day="$(date -u +%F)"

# 201, затем 200 со списком бюджетов.
curl -fsS -w '\n' "$base/api/budgets" -H 'Content-Type: application/json' \
  -d "{\"category\":\"$category\",\"limit\":500}"
curl -fsS -w '\n' "$base/api/budgets"

# 201 для одиночного расхода, затем 200 с одним принятым расходом из пакета.
curl -fsS -w '\n' "$base/api/transactions" -H 'Content-Type: application/json' \
  -d "{\"category\":\"$category\",\"amount\":125,\"date\":\"$day\"}"
curl -fsS -w '\n' "$base/api/transactions/bulk?workers=2" -H 'Content-Type: application/json' \
  -d "[{\"category\":\"$category\",\"amount\":75,\"date\":\"$day\"}]"

# Обе транзакции в списке, в отчёте для новой категории — 200 копеек.
curl -fsS -w '\n' "$base/api/transactions"
curl -fsS -w '\n' "$base/api/reports/summary?from=$day&to=$day"
```

При повторной проверке за ту же дату отчёт обновится после истечения кеша
через 30 секунд.

### Ручная сборка образов

```sh
docker build --build-context ledger=./ledger -t hello-go-gateway:local ./gateway
docker build -t hello-go-ledger:local ./ledger
```

Gateway получает исходники соседнего Go-модуля через контекст `ledger`.
Отдельный образ Ledger содержит демонстрационную программу из `cmd/demo`.

Для проверки Gateway через `docker run` сначала примените миграции из шага 2.
В каталоге клона `hello-go` с именем Compose-проекта по умолчанию выполните:

```sh
docker compose up -d --wait db redis
docker compose stop gateway
docker run --rm --network hello-go_default -p 8080:8080 \
  -e 'DATABASE_URL=postgres://postgres:postgres@db:5432/cashapp?sslmode=disable' \
  -e 'REDIS_ADDR=redis:6379' hello-go-gateway:local
```

В другом терминале вызовите `curl -fsS http://localhost:8080/ping`.
Остановите ручной запуск через Ctrl+C. Чтобы остановить стенд, выполните
`docker compose down`. Данные PostgreSQL сохраняются в томе `postgres_data`.

## Структура

```text
ledger/
  ledger.go                 интерфейс Service и публичные типы
  app.go                    фабрика Open и закрытие ресурсов
  internal/
    domain/                 сущности, валидация, контракты репозиториев
    service/                проверка бюджета, работа с расходами и отчётами
    repository/pg/          SQL-запросы и SQL-транзакции
    db/                     настройки подключения PostgreSQL
    cache/                  подключение Redis, кеш отчётов и списка бюджетов
  migrations/               миграции Goose, запускаются отдельно
gateway/
  main.go                   запуск и остановка сервера
  internal/api/             DTO, обработчики и HTTP-ошибки
```

Домен не знает о SQL и HTTP. Сервис работает с доменными интерфейсами,
репозитории реализуют их для PostgreSQL. Конкретные зависимости собираются
в `ledger.Open(ctx)`: фабрика читает окружение, проверяет подключения через
Ping с переданным контекстом и возвращает сервис, функцию закрытия и ошибку.
Глобальных подключений в приложении нет.

В Gateway сервис передаётся в конструктор обработчиков:

```go
service, closeFn, err := ledger.Open(ctx)
if err != nil {
    return err
}
defer closeFn()
handler := api.NewHandler(service)
```

Gateway импортирует только публичный пакет Ledger. Обработчики вызывают
`Service`, передают контекст запроса и преобразуют результат в прежние DTO.
Валидация и правило лимита выполняются в сервисе. Для проверки и записи расхода
он использует `TransactionRunner`: репозитории внутри callback работают
в одной SQL-транзакции, а строка бюджета блокируется до её завершения.
Так сохраняется защита от превышения лимита при параллельных запросах.

## Проверка

Без БД можно запустить те же команды `go test -race ./...` в `ledger` и `gateway`.
Unit-тесты сервиса проверяют лимит, ошибки хранения и недоступный кеш без
PostgreSQL и Redis. Валидация сущностей и HTTP-маршруты тоже проверяются отдельно.

Для тестов с БД нужны работающие PostgreSQL и Redis:

```sh
export TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/cashapp?sslmode=disable'
export TEST_REDIS_ADDR='localhost:6379'
cd ledger
go test -race ./...
cd ../gateway
go test -race ./...
```

Без `TEST_DATABASE_URL` тесты с БД пропускаются. Каждый такой тест создаёт
отдельную схему PostgreSQL; для кеша используется Redis DB 15, а общий ключ
`budgets:all` очищается до и после каждого теста.
Проверка через настоящий HTTP-сервер и его перезапуск из корня проекта:

```sh
python3 scripts/smoke.py
```
