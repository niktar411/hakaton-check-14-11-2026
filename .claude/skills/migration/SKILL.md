---
name: migration
description: Изменить схему базы через goose (новая миграция, Up и Down, применение на dev, sqlc generate). Использовать при любой новой таблице, колонке, индексе или переносе данных.
---

1. **Новая миграция генератором** (закоммиченные не правят никогда — хук заблокирует):
   ```bash
   bun run new:migration <описание_snake_case>     # add_jobs, add_jobs_city_index
   ```
2. **Up и Down** — обе секции. Down действительно откатывает Up.
3. Правила схемы — `.claude/rules/sql.md`: identity-ключ, `created_at timestamptz`, FK с `on delete`
   и индексом, деньги в копейках `bigint`. Перенос данных — отдельной миграцией.
4. **Применить на dev:** `goose -dir server/migrations postgres "$DATABASE_URL" up`, проверить
   откат: `down`, затем снова `up`.
5. **Запросы:** `server/queries/<ресурс>.sql` → `sqlc generate` в `server/`.
6. Изменился ответ API → поправь `types.go` и `bun run gen:api`.

Нет Postgres или goose локально — скажи об этом прямо; миграцию применит деплой, а проверит CI.
