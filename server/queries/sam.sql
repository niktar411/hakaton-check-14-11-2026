-- name: SearchSamProducts :many
SELECT p.*
FROM sam_products p
WHERE p.name ILIKE '%' || sqlc.arg('query')::text || '%'
   OR p.vendor ILIKE '%' || sqlc.arg('query')::text || '%'
   OR EXISTS (
       SELECT 1 FROM sam_product_aliases a
       WHERE a.product_id = p.id
         AND (a.alias ILIKE '%' || sqlc.arg('query')::text || '%' OR a.normalized_alias LIKE '%' || lower(sqlc.arg('query')::text) || '%')
   )
ORDER BY p.name, p.id
LIMIT sqlc.arg('limit')::int;

-- name: ListSamProducts :many
SELECT *
FROM sam_products
WHERE sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text
ORDER BY name, id
LIMIT sqlc.arg('limit')::int;

-- name: SamProductBalances :many
SELECT p.id,
       p.name,
       p.vendor,
       p.version,
       p.category,
       p.status,
       coalesce(licensed.seats, 0)::bigint AS licensed_seats,
       coalesce(licensed.expired_seats, 0)::bigint AS expired_seats,
       (
           SELECT count(*) FROM sam_installations i
           WHERE i.product_id = p.id AND i.status = 'installed'
       )::bigint AS installed,
       greatest(
           coalesce(licensed.seats, 0) - (
               SELECT count(*) FROM sam_installations i
               WHERE i.product_id = p.id AND i.status = 'installed'
           ),
           0
       )::bigint AS available_seats,
       licensed.nearest_expiry
FROM sam_products p
LEFT JOIN LATERAL (
    SELECT coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND l.expires_at < current_date
           ), 0)::bigint AS expired_seats,
           min(l.expires_at) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ) AS nearest_expiry
    FROM sam_licenses l
    WHERE l.product_id = p.id
) licensed ON true
WHERE p.name ILIKE '%' || sqlc.arg('query')::text || '%'
   OR p.vendor ILIKE '%' || sqlc.arg('query')::text || '%'
   OR EXISTS (
       SELECT 1 FROM sam_product_aliases a
       WHERE a.product_id = p.id
         AND (a.alias ILIKE '%' || sqlc.arg('query')::text || '%' OR a.normalized_alias LIKE '%' || lower(sqlc.arg('query')::text) || '%')
   )
ORDER BY p.name, p.id
LIMIT sqlc.arg('limit')::int;

-- name: SamFindInstallations :many
SELECT i.id,
       i.raw_name,
       i.version,
       i.status,
       i.last_seen_at,
       coalesce(p.name, '')::text AS product_name
FROM sam_installations i
LEFT JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed'
  AND (NOT sqlc.arg('unmatched')::boolean OR i.product_id IS NULL)
  AND (sqlc.arg('product_id')::bigint = 0 OR i.product_id = sqlc.arg('product_id')::bigint)
  AND (sqlc.arg('query')::text = '' OR i.raw_name ILIKE '%' || sqlc.arg('query')::text || '%')
ORDER BY i.last_seen_at DESC, i.id
LIMIT sqlc.arg('limit')::int;

-- name: SamLicenseSummary :many
SELECT l.license_type,
       count(*)::bigint AS licenses,
       coalesce(sum(l.seats), 0)::bigint AS seats,
       count(*) FILTER (WHERE l.expires_at IS NOT NULL AND l.expires_at < current_date)::bigint AS expired,
       count(DISTINCT l.product_id)::bigint AS products,
       coalesce(sum(l.seats * l.cost_per_seat), 0)::bigint AS annual_cost
FROM sam_licenses l
WHERE l.status = 'active'
GROUP BY l.license_type
ORDER BY l.license_type;

-- name: SamTopRisks :many
SELECT risk, severity, affected, detail
FROM sam_risk_overview
ORDER BY CASE severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, affected DESC, risk
LIMIT sqlc.arg('limit')::int;

-- name: SamUnmatchedNames :many
SELECT raw_name,
       count(*)::bigint AS installations,
       count(DISTINCT device_name)::bigint AS devices,
       max(last_seen_at) AS last_seen
FROM sam_installations
WHERE product_id IS NULL AND status = 'installed'
GROUP BY raw_name
ORDER BY installations DESC, raw_name
LIMIT sqlc.arg('limit')::int;

-- name: SamOverview :one
SELECT
    (SELECT count(*) FROM sam_products)::bigint AS products,
    (SELECT count(*) FROM sam_products WHERE status = 'active')::bigint AS active_products,
    (SELECT count(*) FROM sam_installations WHERE status = 'installed')::bigint AS installations,
    (SELECT count(*) FROM sam_installations WHERE product_id IS NULL AND status = 'installed')::bigint AS unmatched_installations,
    (SELECT count(*) FROM sam_licenses WHERE status = 'active')::bigint AS active_licenses,
    (SELECT coalesce(sum(seats), 0) FROM sam_licenses WHERE status = 'active')::bigint AS licensed_seats,
    (SELECT count(*) FROM sam_licenses WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at < current_date)::bigint AS expired_licenses;

-- name: CreateSamAIConversation :one
INSERT INTO sam_ai_conversations(actor, mode)
VALUES ($1, $2)
RETURNING *;

-- name: GetSamAIConversation :one
SELECT *
FROM sam_ai_conversations
WHERE id = $1 AND actor = $2 AND mode = $3;

-- name: ListSamAIConversations :many
SELECT c.*,
       coalesce((
           SELECT m.content FROM sam_ai_messages m
           WHERE m.conversation_id = c.id AND m.role = 'user'
           ORDER BY m.id LIMIT 1
       ), '')::text AS title,
       (
           SELECT count(*) FROM sam_ai_messages m
           WHERE m.conversation_id = c.id
       )::bigint AS message_count
FROM sam_ai_conversations c
WHERE c.actor = $1 AND c.mode = $2
ORDER BY c.updated_at DESC, c.id DESC
LIMIT $3 OFFSET $4;

-- name: ListSamAIMessages :many
SELECT *
FROM sam_ai_messages
WHERE conversation_id = $1
ORDER BY id
LIMIT 200;

-- name: AddSamAIMessage :exec
INSERT INTO sam_ai_messages(conversation_id, role, content, tool_name)
VALUES ($1, $2, $3, $4);

-- name: TouchSamAIConversation :exec
UPDATE sam_ai_conversations SET updated_at = now() WHERE id = $1;

-- name: DeleteEmptySamAIConversations :exec
DELETE FROM sam_ai_conversations
WHERE actor = $1 AND mode = $2
  AND created_at < now() - interval '10 minutes'
  AND NOT EXISTS (SELECT 1 FROM sam_ai_messages WHERE sam_ai_messages.conversation_id = sam_ai_conversations.id);
