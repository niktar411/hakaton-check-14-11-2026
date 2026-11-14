-- name: CreateSamAIAction :one
INSERT INTO sam_ai_actions(conversation_id, actor, intent, payload, preview, status, affected, fingerprint)
VALUES ($1, $2, $3, $4, $5, 'proposed', $6, $7)
RETURNING *;

-- name: GetSamAIAction :one
SELECT *
FROM sam_ai_actions
WHERE id = $1;

-- name: GetSamAIActionForUpdate :one
SELECT *
FROM sam_ai_actions
WHERE id = $1
FOR UPDATE;

-- name: ConfirmSamAIAction :execrows
UPDATE sam_ai_actions
SET status = 'confirmed'
WHERE id = $1 AND status = 'proposed';

-- name: ApplySamAIAction :execrows
UPDATE sam_ai_actions
SET status = 'applied', affected = $2, before = $3, after = $4, error = '', applied_at = now()
WHERE id = $1 AND status = 'confirmed';

-- name: FailSamAIAction :execrows
UPDATE sam_ai_actions
SET status = 'failed', error = $2
WHERE id = $1 AND status IN ('proposed', 'confirmed');

-- name: CancelSamAIAction :execrows
UPDATE sam_ai_actions
SET status = 'cancelled'
WHERE id = $1 AND status = 'proposed';

-- name: UndoSamAIAction :execrows
UPDATE sam_ai_actions
SET status = 'cancelled', error = 'undone', undone_at = now()
WHERE id = $1 AND status = 'applied';

-- name: ListSamAIActions :many
SELECT *
FROM sam_ai_actions
WHERE (sqlc.arg('actor')::text = '' OR actor = sqlc.arg('actor')::text)
  AND (sqlc.arg('conversation_id')::bigint = 0 OR conversation_id = sqlc.arg('conversation_id')::bigint)
  AND (sqlc.arg('status')::text = '' OR status = sqlc.arg('status')::text)
ORDER BY id DESC
LIMIT sqlc.arg('limit')::int OFFSET sqlc.arg('offset')::int;

-- name: LatestAppliedSamAIAction :one
SELECT *
FROM sam_ai_actions
WHERE status = 'applied'
ORDER BY applied_at DESC NULLS LAST, id DESC
LIMIT 1;

-- name: SamProductsByRefs :many
SELECT p.*
FROM sam_products p
WHERE (coalesce(cardinality(sqlc.arg('ids')::bigint[]), 0) = 0 OR p.id = ANY(sqlc.arg('ids')::bigint[]))
  AND (
      sqlc.arg('query')::text = ''
      OR p.name ILIKE '%' || sqlc.arg('query')::text || '%'
      OR p.vendor ILIKE '%' || sqlc.arg('query')::text || '%'
      OR EXISTS (
          SELECT 1 FROM sam_product_aliases a
          WHERE a.product_id = p.id
            AND (a.alias ILIKE '%' || sqlc.arg('query')::text || '%'
                 OR a.normalized_alias LIKE '%' || lower(sqlc.arg('query')::text) || '%')
      )
  )
ORDER BY p.name, p.id
LIMIT sqlc.arg('limit')::int;

-- name: SamProductsByIDsForUpdate :many
SELECT *
FROM sam_products
WHERE id = ANY(sqlc.arg('ids')::bigint[])
ORDER BY id
FOR UPDATE;

-- name: SamProductByID :one
SELECT *
FROM sam_products
WHERE id = $1;

-- name: SamProductNameExists :one
SELECT EXISTS(SELECT 1 FROM sam_products WHERE lower(name) = lower(sqlc.arg('name')::text)) AS exists;

-- name: SamUpdateProductStatus :execrows
UPDATE sam_products
SET status = sqlc.arg('status')::text, updated_at = now()
WHERE id = ANY(sqlc.arg('ids')::bigint[]) AND status <> sqlc.arg('status')::text;

-- name: SamUpdateProductCategory :execrows
UPDATE sam_products
SET category = sqlc.arg('category')::text, updated_at = now()
WHERE id = ANY(sqlc.arg('ids')::bigint[]) AND category <> sqlc.arg('category')::text;

-- name: SamArchiveProducts :execrows
UPDATE sam_products
SET status = 'archived', updated_at = now()
WHERE id = ANY($1::bigint[]) AND status <> 'archived';

-- name: SamRestoreProductStatuses :execrows
UPDATE sam_products p
SET status = v.status, updated_at = now()
FROM unnest(sqlc.arg('ids')::bigint[]) WITH ORDINALITY AS u(id, ord)
JOIN unnest(sqlc.arg('statuses')::text[]) WITH ORDINALITY AS v(status, ord) ON v.ord = u.ord
WHERE p.id = u.id;

-- name: SamRestoreProductCategories :execrows
UPDATE sam_products p
SET category = v.category, updated_at = now()
FROM unnest(sqlc.arg('ids')::bigint[]) WITH ORDINALITY AS u(id, ord)
JOIN unnest(sqlc.arg('categories')::text[]) WITH ORDINALITY AS v(category, ord) ON v.ord = u.ord
WHERE p.id = u.id;

-- name: SamCreateProduct :one
INSERT INTO sam_products(name, vendor, version, category, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: SamDeleteProduct :execrows
DELETE FROM sam_products p
WHERE p.id = $1
  AND NOT EXISTS (SELECT 1 FROM sam_installations i WHERE i.product_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM sam_licenses l WHERE l.product_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM sam_product_aliases a WHERE a.product_id = p.id);

-- name: SamAliasConflicts :many
SELECT normalized_alias, product_id
FROM sam_product_aliases
WHERE normalized_alias = ANY(sqlc.arg('normalized_aliases')::text[])
ORDER BY normalized_alias;

-- name: SamAliasesForProduct :many
SELECT *
FROM sam_product_aliases
WHERE product_id = $1
ORDER BY id;

-- name: SamCreateAliases :many
INSERT INTO sam_product_aliases(product_id, alias, normalized_alias, created_by)
SELECT sqlc.arg('product_id')::bigint, a.alias, n.normalized_alias, sqlc.arg('created_by')::text
FROM unnest(sqlc.arg('aliases')::text[]) WITH ORDINALITY AS a(alias, ord)
JOIN unnest(sqlc.arg('normalized_aliases')::text[]) WITH ORDINALITY AS n(normalized_alias, ord) ON n.ord = a.ord
ON CONFLICT (normalized_alias) DO NOTHING
RETURNING *;

-- name: SamDeleteAliases :execrows
DELETE FROM sam_product_aliases
WHERE id = ANY(sqlc.arg('ids')::bigint[]);

-- name: SamUnmatchedNamesMatching :many
SELECT raw_name,
       count(*)::bigint AS installations,
       count(DISTINCT device_name)::bigint AS devices,
       max(last_seen_at) AS last_seen
FROM sam_installations
WHERE product_id IS NULL AND status = 'installed'
  AND EXISTS (
      SELECT 1 FROM unnest(sqlc.arg('patterns')::text[]) AS p
      WHERE lower(raw_name) LIKE '%' || lower(p) || '%'
  )
GROUP BY raw_name
ORDER BY installations DESC, raw_name
LIMIT sqlc.arg('limit')::int;

-- name: SamInstallationsByRawNames :many
SELECT id, product_id, raw_name, status
FROM sam_installations
WHERE raw_name = ANY(sqlc.arg('raw_names')::text[]) AND status = 'installed'
ORDER BY id;

-- name: SamInstallationsByRawNamesForUpdate :many
SELECT id, product_id, raw_name, status
FROM sam_installations
WHERE raw_name = ANY(sqlc.arg('raw_names')::text[]) AND status = 'installed'
ORDER BY id
FOR UPDATE;

-- name: SamSetInstallationProduct :execrows
UPDATE sam_installations
SET product_id = sqlc.arg('product_id')::bigint
WHERE id = ANY(sqlc.arg('ids')::bigint[])
  AND status = 'installed'
  AND product_id IS DISTINCT FROM sqlc.arg('product_id')::bigint;

-- name: SamClearInstallationProduct :execrows
UPDATE sam_installations
SET product_id = NULL
WHERE id = ANY(sqlc.arg('ids')::bigint[]);

-- name: SamLicensesByRefs :many
SELECT l.*
FROM sam_licenses l
WHERE (coalesce(cardinality(sqlc.arg('ids')::bigint[]), 0) = 0 OR l.id = ANY(sqlc.arg('ids')::bigint[]))
  AND (coalesce(cardinality(sqlc.arg('product_ids')::bigint[]), 0) = 0 OR l.product_id = ANY(sqlc.arg('product_ids')::bigint[]))
ORDER BY l.id;

-- name: SamLicensesForUpdate :many
SELECT l.*
FROM sam_licenses l
WHERE (coalesce(cardinality(sqlc.arg('ids')::bigint[]), 0) = 0 OR l.id = ANY(sqlc.arg('ids')::bigint[]))
  AND (coalesce(cardinality(sqlc.arg('product_ids')::bigint[]), 0) = 0 OR l.product_id = ANY(sqlc.arg('product_ids')::bigint[]))
ORDER BY l.id
FOR UPDATE;

-- name: SamUpdateLicense :execrows
UPDATE sam_licenses
SET license_type = $2,
    seats = $3,
    cost_per_seat = $4,
    currency = $5,
    starts_at = $6,
    expires_at = $7,
    status = $8,
    updated_at = now()
WHERE id = $1
  AND (license_type IS DISTINCT FROM $2
       OR seats <> $3
       OR cost_per_seat <> $4
       OR currency <> $5
       OR starts_at IS DISTINCT FROM $6
       OR expires_at IS DISTINCT FROM $7
       OR status <> $8);
