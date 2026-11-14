-- name: ListChatNotifications :many
SELECT n.id, n.kind, n.title, n.body, n.created_at, n.acknowledged_at
FROM chat_notifications n JOIN local_accounts a ON a.id = n.user_id
WHERE a.username = $1
ORDER BY n.created_at DESC, n.id DESC
LIMIT $2 OFFSET $3;

-- name: CreateChatNotification :execrows
INSERT INTO chat_notifications(user_id, kind, dedup_key, title, body)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, dedup_key) DO NOTHING;

-- name: AcknowledgeChatNotification :execrows
UPDATE chat_notifications n SET acknowledged_at = now()
WHERE n.id = $1
  AND n.user_id = (SELECT a.id FROM local_accounts a WHERE a.username = $2)
  AND n.acknowledged_at IS NULL;

-- name: ListEnabledLocalAccounts :many
SELECT id, username, display_name FROM local_accounts WHERE enabled ORDER BY id;
