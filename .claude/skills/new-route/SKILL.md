---
name: new-route
description: Добавить новый ресурс или эндпоинт API на Go (генератор, huma-типы с валидацией, сервис с правами, sqlc-запросы, тест, регистрация, gen:api). Использовать, когда нужен новый эндпоинт или ресурс в server/internal/.
---

1. **Скелет генератором:**
   ```bash
   bun run new:route <ресурс>        # jobs, user-settings…
   ```
   Создаёт `server/internal/<пакет>/{types,service,handler,handler_test}.go` с рабочим `GET /api/<ресурс>/{id}`
   и регистрирует в `server/internal/http/router.go` (метка `@new-route`). Что не смог — напечатает.
2. **Контракт** — в `types.go`: поля ответа, входы с тегами huma (`path`, `query`, `json`,
   `minLength`, `maxLength`, `pattern`, `format`, `enum`, `minimum`). Это единственный источник
   правды для клиента.
3. **Хранилище:** таблицы нет — `/migration`. Запросы — `server/queries/<ресурс>.sql`
   (`-- name: … :one|:many|:exec`), затем `sqlc generate`. Сервис принимает интерфейс `Store`,
   реализованный sqlc-методами.
4. **Права** — в сервисе: кто может читать/менять ЭТОТ ресурс. Владелец, роль, цена — с сервера.
5. **Ошибки:** доменные (`ErrNotFound`, `ErrForbidden`) → в handler `huma.Error404NotFound("<ресурс>.not_found")`.
   Остальные — `fmt.Errorf("…: %w", err)`.
6. **Тест:** `humatest` с фейковым хранилищем — успех, 404, 403, ошибка валидации.
7. **Контракт клиенту:** `bun run gen:api`.
8. **Проверка:** `go vet ./... && go test ./...` в `server/` (нет Go локально — скажи, что проверит CI).
