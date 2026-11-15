-- name: GetApplicationSetting :one
SELECT * FROM application_settings WHERE key=$1;
-- name: UpsertApplicationSetting :one
INSERT INTO application_settings(key,value) VALUES ($1,$2)
ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now() RETURNING *;
-- name: AddFAQEntry :one
INSERT INTO faq_entries(question,answer,links) VALUES ($1,$2,$3) RETURNING *;
-- name: ListFAQEntries :many
SELECT * FROM faq_entries ORDER BY id DESC LIMIT $1 OFFSET $2;
