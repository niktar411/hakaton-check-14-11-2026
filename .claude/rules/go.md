---
paths:
  - "server/**/*.go"
---

# Сервер: Go

**Слои:** `handler.go` (huma: вход/выход, коды ответа) → `service.go` (логика, права) →
хранилище (sqlc-код из `internal/db/sqlc`). Handler не ходит в базу напрямую.
Новый ресурс — `bun run new:route <ресурс>`.

**Контракт:** типы запросов и ответов — в `types.go` с тегами huma (`minLength`, `maxLength`,
`pattern`, `format:"email"`, `enum`, `minimum`). Это единственный источник: клиент получает
их через `bun run gen:api`. Поменял типы — перегенерируй.

**Ошибки:**
- Оборачивай с контекстом: `return fmt.Errorf("load job %d: %w", id, err)`.
- Доменные ошибки — `var ErrNotFound = errors.New(...)`, проверка `errors.Is`.
- В handler — `huma.Error404NotFound("job.not_found")`: сообщение — **код для i18n**, клиент переводит сам.
- Никогда: `_ = err`, `if err != nil { return nil }`, `panic` в обработке запроса.

**Контекст:** `ctx context.Context` первым аргументом в каждой функции с I/O.
`context.Background()` — только в `cmd/`. Тяжёлая работа — в горутине с `ctx` и семафором.

**Логи:** `slog.InfoContext(ctx, "job created", "job_id", id)` — сообщение + пары ключ/значение.
`fmt.Print*`, `log.*` запрещены. Секреты, пароли, токены и ПДн не логируем.

**Безопасность:** права проверяет сервис, а не клиент. Цена, роль, владелец — только с сервера,
никогда из тела запроса. SQL — только через sqlc (параметры), без склейки строк.

**Тесты:** `humatest` для handler'ов (фейковое хранилище в `_test.go` — можно);
интеграционные — testcontainers-go. Без базы тест падает, а не пропускается.
