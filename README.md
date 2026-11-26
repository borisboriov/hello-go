# Система аналитики личных трат

Автор: Соскин Борис Яковлевич.

Gateway — HTTP API для учёта расходов. Ledger работает внутри Gateway как
библиотека. Он хранит бюджеты и расходы в PostgreSQL, проверяет лимиты
и кеширует отчёты и список бюджетов в Redis на 30 секунд.

- [Подготовка PostgreSQL, Redis и миграций](ledger/README.md)
- [Запуск Gateway и проверка через cURL](gateway/README.md)
- [ДЗ №10. Пакетный импорт транзакций](gateway/README.md#пакетный-импорт)

## Запуск в Docker Compose

Нужны Docker с BuildKit, Docker Compose 2.17+ и Goose. [Установка Goose](ledger/README.md#подготовка-бд)

Если ещё работает `hello-go-postgres-1` из прошлого задания, сначала
остановите его. При том же имени Compose-проекта `db` подхватит старый том.

Из корня проекта поднимаем базу, накатываем миграции и запускаем Gateway.
Redis поднимется вместе с ним.

```sh
docker compose up -d --wait db
export DATABASE_URL="postgres://postgres:postgres@127.0.0.1:${POSTGRES_PORT:-5432}/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
docker compose up -d --build --wait gateway
docker compose ps
```

В `docker compose ps` у Gateway должен быть статус `healthy`.

В Compose заданы `POSTGRES_USER=postgres`, `POSTGRES_PASSWORD=postgres`
и `POSTGRES_DB=cashapp`. У Gateway свой `DATABASE_URL` с адресом `db:5432`
и `REDIS_ADDR=redis:6379`. Порты базы и Redis на хосте можно поменять через
`POSTGRES_PORT` и `REDIS_PORT`. Gateway доступен на `localhost:8080`.

### Проверка HTTP

```sh
docker compose exec gateway wget -qO- http://localhost:8080/ping
curl -fsS http://localhost:8080/ping
```

Обе команды вернут `{"message":"pong"}`. Создадим бюджет и добавим два расхода,
один обычным запросом, другой через пакетный импорт.

```sh
curl -i http://localhost:8080/api/budgets \
  -H 'Content-Type: application/json' -d '{"category":"docker-demo","limit":500}'
curl -i http://localhost:8080/api/budgets

curl -i http://localhost:8080/api/transactions \
  -H 'Content-Type: application/json' \
  -d '{"category":"docker-demo","amount":125,"date":"2026-11-26"}'
curl -i http://localhost:8080/api/transactions/bulk \
  -H 'Content-Type: application/json' \
  -d '[{"category":"docker-demo","amount":75,"date":"2026-11-26"}]'
curl -i http://localhost:8080/api/transactions
curl -i 'http://localhost:8080/api/reports/summary?from=2026-11-26&to=2026-11-26'
```

В отчёте для `docker-demo` должно быть 200 копеек. Если проверяете ещё раз,
возьмите другую категорию и дату, чтобы старые расходы и кеш не мешали сверке.

Стенд останавливается командой `docker compose down`. База остаётся в томе.

### Ручная сборка образов

```sh
docker build --build-context ledger=./ledger -t hello-go-gateway:local ./gateway
docker build -t hello-go-ledger:local ./ledger
```

Контекст `ledger` нужен из-за `replace` в `gateway/go.mod`. В образ Ledger
собирается демо из `cmd/demo`.

Перед `docker run` примените миграции, как показано выше. Для обычного
клона в каталоге `hello-go` команда выглядит так.

```sh
docker compose up -d --wait db redis
docker compose stop gateway
docker run --rm --network hello-go_default -p 8080:8080 \
  -e 'DATABASE_URL=postgres://postgres:postgres@db:5432/cashapp?sslmode=disable' \
  -e 'REDIS_ADDR=redis:6379' hello-go-gateway:local
```

Проверьте `/ping` из другого терминала. Чтобы остановить контейнер, нажмите Ctrl+C.

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
