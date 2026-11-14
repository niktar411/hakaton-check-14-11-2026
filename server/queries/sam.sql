-- name: SamDashboard :one
SELECT
    (SELECT count(*) FROM sam_products)::bigint AS products,
    (SELECT count(*) FROM sam_assets)::bigint AS assets,
    (SELECT count(*) FROM sam_licenses)::bigint AS licenses,
    (SELECT coalesce(sum(seats_total), 0) FROM sam_licenses)::bigint AS seats_total,
    (SELECT coalesce(sum(seats_used), 0) FROM sam_licenses)::bigint AS seats_used,
    (SELECT count(*) FROM sam_findings WHERE status <> 'resolved')::bigint AS open_findings,
    (SELECT count(*) FROM sam_matches WHERE status = 'suggested')::bigint AS pending_matches,
    (SELECT count(*) FROM sam_licenses
        WHERE valid_until IS NOT NULL
          AND valid_until >= current_date
          AND valid_until <= current_date + 30)::bigint AS expiring_licenses,
    (SELECT count(*) FROM sam_installations WHERE product_id IS NULL)::bigint AS unlicensed_installations;

-- name: ListSamProducts :many
SELECT p.id,
       p.name,
       p.vendor,
       p.category,
       p.updated_at,
       (SELECT count(*) FROM sam_licenses l WHERE l.product_id = p.id)::bigint AS license_count,
       (SELECT count(*) FROM sam_installations i WHERE i.product_id = p.id)::bigint AS installation_count
FROM sam_products p
ORDER BY lower(p.name), p.id
LIMIT $1 OFFSET $2;

-- name: ListSamLicenses :many
SELECT l.id,
       p.name AS product_name,
       p.vendor AS product_vendor,
       l.kind,
       l.seats_total,
       l.seats_used,
       l.valid_until,
       CASE
           WHEN l.valid_until IS NULL THEN 'active'
           WHEN l.valid_until < current_date THEN 'expired'
           WHEN l.valid_until <= current_date + 30 THEN 'expiring'
           ELSE 'active'
       END::text AS status
FROM sam_licenses l
JOIN sam_products p ON p.id = l.product_id
ORDER BY l.valid_until NULLS LAST, lower(p.name), l.id
LIMIT $1 OFFSET $2;

-- name: ListSamAssets :many
SELECT a.id,
       a.hostname,
       a.platform,
       a.owner,
       a.last_seen_at,
       (SELECT count(*) FROM sam_installations i WHERE i.asset_id = a.id)::bigint AS software_count
FROM sam_assets a
ORDER BY lower(a.hostname), a.id
LIMIT $1 OFFSET $2;

-- name: ListSamFindings :many
SELECT f.id,
       f.code,
       f.severity,
       f.status,
       f.details,
       f.detected_at,
       coalesce(p.name, '') AS product_name,
       coalesce(a.hostname, '') AS asset_hostname
FROM sam_findings f
LEFT JOIN sam_products p ON p.id = f.product_id
LEFT JOIN sam_assets a ON a.id = f.asset_id
ORDER BY CASE f.severity
             WHEN 'critical' THEN 0
             WHEN 'high' THEN 1
             WHEN 'medium' THEN 2
             ELSE 3
         END,
         f.detected_at DESC,
         f.id DESC
LIMIT $1 OFFSET $2;

-- name: ListSamMatches :many
SELECT m.id,
       i.detected_name,
       i.version,
       a.hostname AS asset_hostname,
       coalesce(p.name, '') AS product_name,
       m.confidence,
       m.status,
       m.created_at
FROM sam_matches m
JOIN sam_installations i ON i.id = m.installation_id
JOIN sam_assets a ON a.id = i.asset_id
LEFT JOIN sam_products p ON p.id = m.product_id
ORDER BY m.created_at DESC, m.id DESC
LIMIT $1 OFFSET $2;
