-- name: CreateLocalAccount :one
INSERT INTO local_accounts(username,display_name,password_hash,role,color,personal_context,tone)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: UpsertOAuthAccount :one
INSERT INTO local_accounts(username,display_name,role,color)
VALUES ($1,$1,'member',$2)
ON CONFLICT(username) DO UPDATE SET display_name=EXCLUDED.display_name,updated_at=now()
RETURNING *;
-- name: GetLocalAccountByUsername :one
SELECT * FROM local_accounts WHERE username=$1;
-- name: GetLocalAccountByID :one
SELECT * FROM local_accounts WHERE id=$1;
-- name: ListLocalAccounts :many
SELECT * FROM local_accounts ORDER BY username LIMIT $1 OFFSET $2;
-- name: UpdateLocalAccountProfile :one
UPDATE local_accounts SET display_name=$2,color=$3,personal_context=$4,tone=$5,updated_at=now()
WHERE id=$1 RETURNING *;
-- name: SetLocalAccountEnabled :one
UPDATE local_accounts SET enabled=$2,updated_at=now() WHERE id=$1 RETURNING *;
-- name: SetLocalAccountPassword :exec
UPDATE local_accounts SET password_hash=$2,updated_at=now() WHERE id=$1;
-- name: DeleteSessionsByUsername :exec
DELETE FROM sessions WHERE username=$1;
-- name: ListAccountConversations :many
SELECT * FROM conversations WHERE username=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: GetConversationForAdmin :one
SELECT * FROM conversations WHERE id=$1;
-- name: DeleteExpiredOAuthStates :exec
DELETE FROM oauth_states WHERE expires_at<=now();
