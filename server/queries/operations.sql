-- name: CreateSession :exec
INSERT INTO sessions(id,username,access_token,csrf_token,expires_at,account_id) VALUES ($1,$2,$3,$4,$5,$6);
-- name: GetSession :one
SELECT * FROM sessions WHERE id=$1 AND expires_at>now();
-- name: DeleteSession :exec
DELETE FROM sessions WHERE id=$1;
-- name: DeleteSessionsByAccount :exec
DELETE FROM sessions WHERE account_id=$1;
-- name: CreateOAuthState :exec
INSERT INTO oauth_states(id,verifier,expires_at) VALUES ($1,$2,$3);
-- name: ConsumeOAuthState :one
DELETE FROM oauth_states WHERE id=$1 AND expires_at>now() RETURNING verifier;
-- name: CreateConversation :one
INSERT INTO conversations(username,phase) VALUES ($1,'clarifying') RETURNING *;
-- name: GetConversation :one
SELECT * FROM conversations WHERE id=$1 AND username=$2;
-- name: SetConversationReady :exec
UPDATE conversations SET phase='ready' WHERE id=$1 AND username=$2;
-- name: AddMessage :exec
INSERT INTO messages(conversation_id,role,content) VALUES ($1,$2,$3);
-- name: ListMessages :many
SELECT * FROM messages WHERE conversation_id=$1 ORDER BY id LIMIT 80;
-- name: BeginUsage :one
INSERT INTO usage_events(request_id,conversation_id,username,model,status)
VALUES ($1,$2,$3,$4,'pending') RETURNING id;
-- name: FinishUsage :exec
UPDATE usage_events SET status=$2,input_tokens=$3,output_tokens=$4,total_tokens=$5 WHERE id=$1;
-- name: ListUsage :many
SELECT * FROM usage_events ORDER BY id DESC LIMIT $1 OFFSET $2;
-- name: UsageTotals :many
SELECT username, count(*) AS requests, count(*) FILTER(WHERE total_tokens IS NULL) AS unknown_requests,
coalesce(sum(input_tokens),0)::bigint AS input_tokens, coalesce(sum(output_tokens),0)::bigint AS output_tokens,
coalesce(sum(total_tokens),0)::bigint AS total_tokens
FROM usage_events GROUP BY username ORDER BY username;
-- name: BeginAudit :one
INSERT INTO audit_events(username,action,target,status) VALUES ($1,$2,$3,'pending') RETURNING id;
-- name: FinishAudit :exec
UPDATE audit_events SET status=$2 WHERE id=$1;
-- name: ListAudit :many
SELECT * FROM audit_events ORDER BY id DESC LIMIT 100;
