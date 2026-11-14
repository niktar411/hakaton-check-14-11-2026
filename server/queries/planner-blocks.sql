-- name: ListPlannerBlocks :many
SELECT b.*,a.username AS owner_username,a.display_name AS owner_display_name,a.color AS owner_color
FROM planner_blocks b JOIN local_accounts a ON a.id=b.owner_id
ORDER BY b.position,b.id LIMIT $1 OFFSET $2;
-- name: CreatePlannerBlock :one
INSERT INTO planner_blocks(title,description,owner_id,branch,merge_request_iid,position,created_by,parent_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
-- name: NextPlannerPosition :one
SELECT coalesce(max(position)+1,0)::integer AS position FROM planner_blocks;
-- name: GetPlannerBlock :one
SELECT * FROM planner_blocks WHERE id=$1;
-- name: UpdatePlannerBlock :one
UPDATE planner_blocks SET title=$2,description=$3,owner_id=$4,position=$5,parent_id=$6,updated_at=now()
WHERE id=$1 RETURNING *;
-- name: DeletePlannerBlock :exec
DELETE FROM planner_blocks WHERE id=$1;
-- name: SetPlannerBlockMergeRequest :exec
UPDATE planner_blocks SET branch=$2,merge_request_iid=$3,updated_at=now() WHERE id=$1;
-- name: StartPlannerBlock :one
UPDATE planner_blocks SET started_at=coalesce(started_at,now()),updated_at=now()
WHERE id=$1 RETURNING *;
