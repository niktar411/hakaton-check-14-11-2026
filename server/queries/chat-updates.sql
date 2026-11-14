-- name: ListChatPromptRecipients :many
SELECT username FROM local_accounts WHERE enabled AND username<>$1;
-- name: CreateChatPromptIfAbsent :exec
INSERT INTO chat_change_prompts(username,block_id,mr_url)
VALUES ($1,$2,$3) ON CONFLICT (block_id,username) DO NOTHING;
-- name: ListPendingChatPrompts :many
SELECT p.id,p.block_id,p.mr_url,p.created_at,b.title,b.branch,b.description,
a.username AS owner_username,a.display_name AS owner_display_name
FROM chat_change_prompts p
JOIN planner_blocks b ON b.id=p.block_id
JOIN local_accounts a ON a.id=b.owner_id
WHERE p.username=$1 AND p.pulled_at IS NULL
ORDER BY p.created_at,p.id;
-- name: MarkChatPromptPulled :one
UPDATE chat_change_prompts SET pulled_at=now()
WHERE id=$1 AND username=$2 AND pulled_at IS NULL RETURNING id;
-- name: GetChatPrompt :one
SELECT * FROM chat_change_prompts WHERE id=$1;
