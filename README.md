# Учебный проект на Go

Домашнее задание по теме 7: PostgreSQL и Redis.

Ledger хранит бюджеты и расходы в PostgreSQL. Перед добавлением расхода
проверяется лимит категории. Отчёты кешируются в Redis на 30 секунд.
Gateway использует прежние URL и JSON для бюджетов и транзакций.

В проекте три Go-модуля: корневой пример, Ledger и Gateway. Ledger подключён
к Gateway как библиотека, поэтому HTTP-сервер запускается одной командой.

- [Подготовка PostgreSQL, Redis и миграций](ledger/README.md)
- [Запуск Gateway и проверка через cURL](gateway/README.md)

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
отдельную схему PostgreSQL; для кеша используется Redis DB 15.
Проверка через настоящий HTTP-сервер и его перезапуск из корня проекта:

```sh
python3 scripts/smoke.py
```
