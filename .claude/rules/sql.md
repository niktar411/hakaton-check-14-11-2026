---
paths:
  - "server/migrations/**"
  - "server/queries/**"
---

# SQL: миграции и запросы

**Миграции (goose):**
- Новая — только `bun run new:migration <описание_snake_case>`. **Миграцию из текущего коммита HEAD не правят**
  (хук блокирует по политике неизменяемости; состояние БД он не проверяет).
  Новую миграцию можно редактировать и после `git add`, пока её нет в HEAD. Ошибка в старой — исправляется новой миграцией.
- В файле обе секции: `-- +goose Up` и `-- +goose Down`. Down реально откатывает Up.
- Таблицы — множественное число, snake_case; `id bigint generated always as identity primary key`,
  `created_at timestamptz not null default now()`.
- Внешний ключ — с `on delete` (cascade / set null — осознанно) и индексом.
- Деньги — `bigint` в копейках, не `numeric`/`float`.
- Перенос данных и изменение схемы — разными миграциями.

**Запросы (sqlc):**
- `server/queries/<ресурс>.sql`, каждый запрос с аннотацией `-- name: GetJob :one` / `:many` / `:exec`.
- Параметры — `$1` или `sqlc.arg(name)`, никакой склейки строк.
- Списки — всегда с `limit` / `offset` (или keyset), сортировка явная.
- После правки — `sqlc generate` в `server/`; сгенерированный код руками не правят.
