-- name: CreateMergeApproval :exec
INSERT INTO merge_approvals(merge_request_iid,source_branch,target_branch,commit_sha,approved_by)
VALUES ($1,$2,$3,$4,$5) ON CONFLICT(merge_request_iid,commit_sha) DO NOTHING;
-- name: GetMergeApproval :one
SELECT * FROM merge_approvals WHERE merge_request_iid=$1 AND commit_sha=$2 AND target_branch='main';
-- name: BeginDeploymentOperation :one
INSERT INTO deployment_operations(actor,action,target,release,status)
VALUES ($1,$2,$3,$4,'running') RETURNING *;
-- name: FinishDeploymentOperation :exec
UPDATE deployment_operations SET status=$2,backup_name=$3,details=$4,finished_at=now() WHERE id=$1;
-- name: GetDeploymentOperation :one
SELECT * FROM deployment_operations WHERE id=$1;
-- name: ListDeploymentOperations :many
SELECT * FROM deployment_operations ORDER BY id DESC LIMIT $1 OFFSET $2;
-- name: LatestSuccessfulRelease :one
SELECT * FROM deployment_operations WHERE target='production' AND action='deploy' AND status='completed'
ORDER BY id DESC LIMIT 1;
