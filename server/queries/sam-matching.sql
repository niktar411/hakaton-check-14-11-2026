-- name: SamMatchingUnmatchedInstallations :many
SELECT i.id, i.raw_name, i.device_name, i.last_seen_at
FROM sam_installations i
WHERE i.product_id IS NULL AND i.status = 'installed'
ORDER BY i.id
LIMIT $1 OFFSET $2;

-- name: SamMatchingProducts :many
SELECT p.id, p.name, p.vendor, p.status
FROM sam_products p
WHERE p.status <> 'archived'
ORDER BY p.id;

-- name: SamMatchingAliases :many
SELECT a.id, a.product_id, a.alias, a.normalized_alias, a.confidence
FROM sam_product_aliases a
ORDER BY a.id;

-- name: SamMatchingUpsertCandidate :one
INSERT INTO sam_match_candidates(installation_id, product_id, raw_name, score, method, status)
VALUES ($1, $2, $3, $4, $5, 'pending')
ON CONFLICT (installation_id, product_id) DO UPDATE
SET raw_name = EXCLUDED.raw_name,
    score = EXCLUDED.score,
    method = EXCLUDED.method
WHERE sam_match_candidates.status = 'pending'
RETURNING *;

-- name: SamMatchingCandidates :many
SELECT c.id,
       c.installation_id,
       c.product_id,
       c.raw_name,
       c.score,
       c.method,
       c.status,
       c.created_at,
       c.resolved_at,
       c.resolved_by,
       p.name AS product_name,
       i.device_name,
       i.last_seen_at
FROM sam_match_candidates c
JOIN sam_products p ON p.id = c.product_id
JOIN sam_installations i ON i.id = c.installation_id
WHERE (sqlc.arg('status')::text = '' OR c.status = sqlc.arg('status')::text)
ORDER BY CASE c.status WHEN 'pending' THEN 0 ELSE 1 END, c.score DESC, c.id
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: SamMatchingCandidateView :one
SELECT c.id,
       c.installation_id,
       c.product_id,
       c.raw_name,
       c.score,
       c.method,
       c.status,
       c.created_at,
       c.resolved_at,
       c.resolved_by,
       p.name AS product_name,
       i.device_name,
       i.last_seen_at
FROM sam_match_candidates c
JOIN sam_products p ON p.id = c.product_id
JOIN sam_installations i ON i.id = c.installation_id
WHERE c.id = $1;

-- name: SamMatchingCandidateForUpdate :one
SELECT *
FROM sam_match_candidates
WHERE id = $1
FOR UPDATE;

-- name: SamMatchingConfirmCandidate :execrows
UPDATE sam_match_candidates
SET status = 'confirmed', resolved_at = now(), resolved_by = $2
WHERE id = $1 AND status = 'pending';

-- name: SamMatchingRejectCandidate :execrows
UPDATE sam_match_candidates
SET status = 'rejected', resolved_at = now(), resolved_by = $2
WHERE id = $1 AND status = 'pending';

-- name: SamMatchingRejectOtherCandidates :execrows
UPDATE sam_match_candidates
SET status = 'rejected', resolved_at = now(), resolved_by = $2
WHERE installation_id = $1 AND id <> $3 AND status = 'pending';

-- name: SamMatchingAliasProduct :one
SELECT product_id
FROM sam_product_aliases
WHERE normalized_alias = $1;

-- name: SamMatchingUpsertAlias :execrows
INSERT INTO sam_product_aliases(product_id, alias, normalized_alias, created_by, confidence)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (normalized_alias) DO UPDATE
SET confidence = greatest(sam_product_aliases.confidence, EXCLUDED.confidence)
WHERE sam_product_aliases.product_id = EXCLUDED.product_id;

-- name: SamMatchingSetInstallationProduct :execrows
UPDATE sam_installations
SET product_id = $2
WHERE id = $1 AND product_id IS NULL;

-- name: SamMatchingCountUnmatched :one
SELECT count(*)::bigint AS count
FROM sam_installations
WHERE product_id IS NULL AND status = 'installed';

-- name: SamMatchingCountPending :one
SELECT count(*)::bigint AS count
FROM sam_match_candidates
WHERE status = 'pending';

-- name: SamMatchingCountCandidatesByStatus :one
SELECT count(*)::bigint AS count
FROM sam_match_candidates
WHERE status = $1;
