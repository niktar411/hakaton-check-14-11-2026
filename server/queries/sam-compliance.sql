-- name: SamComplianceSummary :one
WITH license_totals AS (
    SELECT l.product_id,
           coalesce(sum(l.seats_total), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS active_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NOT NULL AND l.valid_until < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats_total * l.cost_per_seat) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats_total * l.cost_per_seat)::numeric
               / nullif(sum(l.seats_total), 0)
           ), 0)::bigint AS avg_cost
    FROM sam_licenses l
    GROUP BY l.product_id
),
product_usage AS (
    SELECT p.id,
           p.status,
           coalesce(l.purchased_seats, 0) AS purchased,
           coalesce(l.active_seats, 0) AS licensed,
           coalesce(l.expired_seats, 0) AS expired,
           coalesce(l.annual_cost, 0) AS annual,
           coalesce(l.avg_cost, 0) AS avg_cost,
           coalesce(inst.devices, 0) AS devices
    FROM sam_products p
    LEFT JOIN license_totals l ON l.product_id = p.id
    LEFT JOIN LATERAL (
        SELECT count(DISTINCT i.asset_id)::bigint AS devices
        FROM sam_installations i
        WHERE i.product_id = p.id
    ) inst ON true
)
SELECT
    (SELECT count(*) FROM sam_products)::bigint AS products,
    (SELECT count(*) FROM sam_products WHERE status = 'active')::bigint AS active_products,
    (SELECT count(*) FROM sam_products WHERE status = 'banned')::bigint AS banned_products,
    (SELECT count(*) FROM sam_assets)::bigint AS assets,
    (SELECT count(*) FROM sam_licenses)::bigint AS licenses,
    (SELECT count(*) FROM sam_installations)::bigint AS installations,
    (SELECT count(DISTINCT asset_id) FROM sam_installations)::bigint AS devices,
    (SELECT count(*) FROM sam_installations WHERE product_id IS NULL)::bigint AS unmatched_installations,
    coalesce(sum(purchased) FILTER (WHERE status <> 'banned'), 0)::bigint AS purchased_seats,
    coalesce(sum(licensed) FILTER (WHERE status <> 'banned'), 0)::bigint AS licensed_seats,
    coalesce(sum(expired) FILTER (WHERE status <> 'banned'), 0)::bigint AS expired_seats,
    coalesce(sum(greatest(devices - licensed, 0)) FILTER (WHERE status <> 'banned'), 0)::bigint AS overused_seats,
    coalesce(sum(greatest(licensed - devices, 0)) FILTER (WHERE status <> 'banned'), 0)::bigint AS idle_seats,
    coalesce(sum(annual) FILTER (WHERE status <> 'banned'), 0)::bigint AS annual_cost,
    coalesce(sum(greatest(devices - licensed, 0) * avg_cost) FILTER (WHERE status <> 'banned'), 0)::bigint AS overuse_cost,
    coalesce(sum(greatest(licensed - devices, 0) * avg_cost) FILTER (WHERE status <> 'banned'), 0)::bigint AS idle_cost,
    coalesce(sum(expired * avg_cost) FILTER (WHERE status <> 'banned'), 0)::bigint AS expired_cost,
    coalesce(sum(CASE WHEN status = 'banned' THEN devices ELSE 0 END), 0)::bigint AS banned_installations
FROM product_usage;

-- name: SamComplianceProducts :many
WITH license_totals AS (
    SELECT l.product_id,
           count(*)::bigint AS licenses,
           coalesce(sum(l.seats_total), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS active_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NOT NULL AND l.valid_until < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats_total * l.cost_per_seat) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats_total * l.cost_per_seat)::numeric
               / nullif(sum(l.seats_total), 0)
           ), 0)::bigint AS avg_cost
    FROM sam_licenses l
    GROUP BY l.product_id
)
SELECT p.id,
       p.name,
       p.vendor,
       p.category,
       p.status,
       coalesce(l.licenses, 0)::bigint AS licenses,
       coalesce(l.purchased_seats, 0)::bigint AS purchased_seats,
       coalesce(l.active_seats, 0)::bigint AS licensed_seats,
       coalesce(l.expired_seats, 0)::bigint AS expired_seats,
       coalesce(l.annual_cost, 0)::bigint AS annual_cost,
       coalesce(l.avg_cost, 0)::bigint AS avg_cost_per_seat,
       coalesce(inst.devices, 0)::bigint AS installed_devices,
       coalesce(inst.installations, 0)::bigint AS installations,
       CASE WHEN p.status = 'banned' THEN coalesce(inst.devices, 0) ELSE 0 END::bigint AS banned_installations
FROM sam_products p
LEFT JOIN license_totals l ON l.product_id = p.id
LEFT JOIN LATERAL (
    SELECT count(*)::bigint AS installations,
           count(DISTINCT i.asset_id)::bigint AS devices
    FROM sam_installations i
    WHERE i.product_id = p.id
) inst ON true
ORDER BY CASE p.status WHEN 'banned' THEN 0 WHEN 'archived' THEN 3 ELSE 1 END,
         greatest(coalesce(inst.devices, 0) - coalesce(l.active_seats, 0), 0) DESC,
         p.name,
         p.id;

-- name: SamComplianceProduct :one
WITH license_totals AS (
    SELECT l.product_id,
           count(*)::bigint AS licenses,
           coalesce(sum(l.seats_total), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS active_seats,
           coalesce(sum(l.seats_total) FILTER (
               WHERE l.valid_until IS NOT NULL AND l.valid_until < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats_total * l.cost_per_seat) FILTER (
               WHERE l.valid_until IS NULL OR l.valid_until >= current_date
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats_total * l.cost_per_seat)::numeric
               / nullif(sum(l.seats_total), 0)
           ), 0)::bigint AS avg_cost
    FROM sam_licenses l
    GROUP BY l.product_id
)
SELECT p.id,
       p.name,
       p.vendor,
       p.category,
       p.status,
       coalesce(l.licenses, 0)::bigint AS licenses,
       coalesce(l.purchased_seats, 0)::bigint AS purchased_seats,
       coalesce(l.active_seats, 0)::bigint AS licensed_seats,
       coalesce(l.expired_seats, 0)::bigint AS expired_seats,
       coalesce(l.annual_cost, 0)::bigint AS annual_cost,
       coalesce(l.avg_cost, 0)::bigint AS avg_cost_per_seat,
       coalesce(inst.devices, 0)::bigint AS installed_devices,
       coalesce(inst.installations, 0)::bigint AS installations,
       CASE WHEN p.status = 'banned' THEN coalesce(inst.devices, 0) ELSE 0 END::bigint AS banned_installations
FROM sam_products p
LEFT JOIN license_totals l ON l.product_id = p.id
LEFT JOIN LATERAL (
    SELECT count(*)::bigint AS installations,
           count(DISTINCT i.asset_id)::bigint AS devices
    FROM sam_installations i
    WHERE i.product_id = p.id
) inst ON true
WHERE p.id = $1;

-- name: ListSamProductLicenses :many
SELECT l.id,
       l.kind,
       l.seats_total,
       l.seats_used,
       l.cost_per_seat,
       l.currency,
       l.valid_until,
       CASE
           WHEN l.valid_until IS NULL THEN 'active'
           WHEN l.valid_until < current_date THEN 'expired'
           WHEN l.valid_until <= current_date + 30 THEN 'expiring'
           ELSE 'active'
       END::text AS status
FROM sam_licenses l
WHERE l.product_id = $1
ORDER BY l.valid_until NULLS LAST, l.id;

-- name: ListSamProductHosts :many
SELECT a.id AS asset_id,
       a.hostname,
       a.platform,
       a.owner,
       max(i.version::text)::text AS version,
       max(i.last_seen_at)::timestamptz AS last_seen_at,
       p.status AS product_status,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id
             AND l.seats_total > 0
             AND (l.valid_until IS NULL OR l.valid_until >= current_date)
       ) AS has_active_license,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id
             AND l.valid_until IS NOT NULL
             AND l.valid_until < current_date
       ) AS has_expired_license
FROM sam_installations i
JOIN sam_assets a ON a.id = i.asset_id
JOIN sam_products p ON p.id = i.product_id
WHERE i.product_id = $1::bigint
GROUP BY a.id, a.hostname, a.platform, a.owner, p.id, p.status
ORDER BY lower(a.hostname), a.id;

-- name: GetSamHost :one
SELECT a.id,
       a.hostname,
       a.platform,
       a.owner,
       a.last_seen_at,
       (SELECT count(*) FROM sam_installations i WHERE i.asset_id = a.id)::bigint AS installations,
       (SELECT count(*) FROM sam_installations i WHERE i.asset_id = a.id AND i.product_id IS NULL)::bigint AS unlicensed
FROM sam_assets a
WHERE a.id = $1;

-- name: ListSamHostInstallations :many
SELECT i.id,
       i.product_id,
       i.detected_name,
       i.version,
       p.name AS product_name,
       p.vendor AS product_vendor,
       p.category AS product_category,
       p.status AS product_status,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id
             AND l.seats_total > 0
             AND (l.valid_until IS NULL OR l.valid_until >= current_date)
       ) AS has_active_license,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id
             AND l.valid_until IS NOT NULL
             AND l.valid_until < current_date
       ) AS has_expired_license
FROM sam_installations i
LEFT JOIN sam_products p ON p.id = i.product_id
WHERE i.asset_id = $1
ORDER BY lower(coalesce(p.name, i.detected_name)), i.id;
