-- name: SamComplianceSummary :one
WITH license_totals AS (
    SELECT l.product_id,
           count(*)::bigint AS licenses,
           coalesce(sum(l.seats), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND l.expires_at < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats * l.cost_per_seat) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats * l.cost_per_seat) FILTER (WHERE l.status = 'active')::numeric
               / nullif(sum(l.seats) FILTER (WHERE l.status = 'active'), 0)
           ), 0)::bigint AS avg_cost
    FROM sam_licenses l
    GROUP BY l.product_id
),
product_usage AS (
    SELECT p.id,
           p.status,
           coalesce(l.licenses, 0) AS licenses,
           coalesce(l.purchased_seats, 0) AS purchased,
           coalesce(l.seats, 0) AS licensed,
           coalesce(l.expired_seats, 0) AS expired,
           coalesce(l.annual_cost, 0) AS annual,
           coalesce(l.avg_cost, 0) AS avg_cost,
           coalesce(inst.devices, 0) AS devices
    FROM sam_products p
    LEFT JOIN license_totals l ON l.product_id = p.id
    LEFT JOIN LATERAL (
        SELECT count(DISTINCT coalesce(nullif(i.device_name, ''), 'installation:' || i.id))::bigint AS devices
        FROM sam_installations i
        WHERE i.product_id = p.id AND i.status = 'installed'
    ) inst ON true
)
SELECT
    (SELECT count(*) FROM sam_products)::bigint AS products,
    (SELECT count(*) FROM sam_products WHERE status = 'active')::bigint AS active_products,
    (SELECT count(*) FROM sam_products WHERE status = 'banned')::bigint AS banned_products,
    (SELECT count(*) FROM sam_installations WHERE status = 'installed')::bigint AS installations,
    (SELECT count(DISTINCT coalesce(nullif(device_name, ''), 'installation:' || id))
     FROM sam_installations WHERE status = 'installed')::bigint AS devices,
    (SELECT count(*) FROM sam_installations
     WHERE status = 'installed' AND product_id IS NULL)::bigint AS unmatched_installations,
    coalesce(sum(licenses) FILTER (WHERE status <> 'banned'), 0)::bigint AS licenses,
    coalesce(sum(purchased) FILTER (WHERE status <> 'banned'), 0)::bigint AS purchased_seats,
    coalesce(sum(licensed) FILTER (WHERE status <> 'banned'), 0)::bigint AS licensed_seats,
    coalesce(sum(greatest(devices - licensed, 0)) FILTER (WHERE status <> 'banned'), 0)::bigint AS overused_seats,
    coalesce(sum(greatest(licensed - devices, 0)) FILTER (WHERE status <> 'banned'), 0)::bigint AS idle_seats,
    coalesce(sum(expired) FILTER (WHERE status <> 'banned'), 0)::bigint AS expired_seats,
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
           coalesce(sum(l.seats), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND l.expires_at < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats * l.cost_per_seat) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats * l.cost_per_seat) FILTER (WHERE l.status = 'active')::numeric
               / nullif(sum(l.seats) FILTER (WHERE l.status = 'active'), 0)
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
       coalesce(l.seats, 0)::bigint AS licensed_seats,
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
           count(DISTINCT coalesce(nullif(i.device_name, ''), 'installation:' || i.id))::bigint AS devices
    FROM sam_installations i
    WHERE i.product_id = p.id AND i.status = 'installed'
) inst ON true
ORDER BY CASE p.status WHEN 'banned' THEN 0 WHEN 'archived' THEN 3 ELSE 1 END,
         greatest(coalesce(inst.devices, 0) - coalesce(l.seats, 0), 0) DESC,
         p.name,
         p.id;

-- name: SamComplianceProduct :one
WITH license_totals AS (
    SELECT l.product_id,
           count(*)::bigint AS licenses,
           coalesce(sum(l.seats), 0)::bigint AS purchased_seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS seats,
           coalesce(sum(l.seats) FILTER (
               WHERE l.status = 'active' AND l.expires_at < current_date
           ), 0)::bigint AS expired_seats,
           coalesce(sum(l.seats * l.cost_per_seat) FILTER (
               WHERE l.status = 'active' AND (l.expires_at IS NULL OR l.expires_at >= current_date)
           ), 0)::bigint AS annual_cost,
           coalesce(round(
               sum(l.seats * l.cost_per_seat) FILTER (WHERE l.status = 'active')::numeric
               / nullif(sum(l.seats) FILTER (WHERE l.status = 'active'), 0)
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
       coalesce(l.seats, 0)::bigint AS licensed_seats,
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
           count(DISTINCT coalesce(nullif(i.device_name, ''), 'installation:' || i.id))::bigint AS devices
    FROM sam_installations i
    WHERE i.product_id = p.id AND i.status = 'installed'
) inst ON true
WHERE p.id = $1::bigint;

-- name: ListSamProductLicenses :many
SELECT l.id,
       l.license_type,
       l.seats,
       l.cost_per_seat,
       l.currency,
       l.expires_at,
       CASE
           WHEN l.status <> 'active' THEN 'expired'
           WHEN l.expires_at IS NULL THEN 'active'
           WHEN l.expires_at < current_date THEN 'expired'
           WHEN l.expires_at <= current_date + 30 THEN 'expiring'
           ELSE 'active'
       END::text AS status
FROM sam_licenses l
WHERE l.product_id = $1::bigint
ORDER BY l.expires_at NULLS LAST, l.id;

-- name: ListSamProductHosts :many
SELECT DISTINCT i.device_name,
       max(i.version)::text AS version,
       max(i.last_seen_at)::timestamptz AS last_seen_at,
       p.status AS product_status,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id AND l.status = 'active'
             AND (l.expires_at IS NULL OR l.expires_at >= current_date)
       ) AS has_active_license,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id AND l.status = 'active'
             AND l.expires_at IS NOT NULL AND l.expires_at < current_date
       ) AS has_expired_license
FROM sam_installations i
JOIN sam_products p ON p.id = i.product_id
WHERE i.product_id = $1::bigint
  AND i.status = 'installed'
  AND coalesce(nullif(i.device_name, ''), 'installation:' || i.id) IS NOT NULL
GROUP BY i.device_name, p.status
ORDER BY i.device_name;

-- name: GetSamHost :one
SELECT i.device_name,
       max(i.last_seen_at)::timestamptz AS last_seen_at,
       count(*)::bigint AS installations,
       count(*) FILTER (WHERE i.product_id IS NULL)::bigint AS unlicensed
FROM sam_installations i
WHERE i.status = 'installed' AND i.device_name = $1::text
GROUP BY i.device_name;

-- name: ListSamHostInstallations :many
SELECT i.id,
       i.product_id,
       i.raw_name,
       i.version,
       p.name AS product_name,
       p.vendor AS product_vendor,
       p.category AS product_category,
       p.status AS product_status,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id AND l.status = 'active'
             AND (l.expires_at IS NULL OR l.expires_at >= current_date)
       ) AS has_active_license,
       EXISTS (
           SELECT 1 FROM sam_licenses l
           WHERE l.product_id = p.id AND l.status = 'active'
             AND l.expires_at IS NOT NULL AND l.expires_at < current_date
       ) AS has_expired_license
FROM sam_installations i
LEFT JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed' AND i.device_name = $1::text
ORDER BY lower(coalesce(p.name, i.raw_name)), i.id;
