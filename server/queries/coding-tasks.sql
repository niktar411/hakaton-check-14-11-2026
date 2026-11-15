-- name: CreateCodingTask :one
INSERT INTO coding_tasks(account_id,parent_id,request_text,phase) VALUES ($1,$2,$3,'clarifying') RETURNING *;
-- name: GetCodingTask :one
SELECT * FROM coding_tasks WHERE id=$1;
-- name: GetOwnedCodingTask :one
SELECT * FROM coding_tasks WHERE id=$1 AND account_id=$2;
-- name: ListOwnedCodingTasks :many
SELECT * FROM coding_tasks WHERE account_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: ListCodingTasks :many
SELECT * FROM coding_tasks ORDER BY id DESC LIMIT $1 OFFSET $2;
-- name: SetCodingTaskClarification :one
UPDATE coding_tasks SET phase='awaiting_confirmation',clarification=$2,updated_at=now()
WHERE id=$1 AND phase='clarifying' RETURNING *;
-- name: StartCodingTask :one
UPDATE coding_tasks SET phase='running',branch=$2,failure_reason='',updated_at=now()
WHERE id=$1 AND account_id=$3 AND phase='awaiting_confirmation' RETURNING *;
-- name: SetCodingTaskMergeRequest :one
UPDATE coding_tasks SET phase='awaiting_merge',merge_request_iid=$2,merge_request_url=$3,updated_at=now()
WHERE id=$1 AND phase='running' RETURNING *;
-- name: SetCodingTaskPhase :one
UPDATE coding_tasks SET phase=$2,failure_reason=$3,updated_at=now() WHERE id=$1 RETURNING *;
-- name: AddCodingMessage :one
INSERT INTO coding_messages(task_id,role,content,tool_name) VALUES ($1,$2,$3,$4) RETURNING *;
-- name: ListCodingMessages :many
SELECT * FROM coding_messages WHERE task_id=$1 ORDER BY id LIMIT $2 OFFSET $3;
