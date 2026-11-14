-- +goose Up
ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'inactive', 'archived', 'banned'));

CREATE OR REPLACE VIEW sam_risk_overview AS
SELECT 'unmatched_installations'::text AS risk,
       'high'::text AS severity,
       count(*)::bigint AS affected,
       'Установки без сопоставленного продукта'::text AS detail
FROM sam_installations
WHERE product_id IS NULL AND status = 'installed'
UNION ALL
SELECT 'expired_licenses',
       'high',
       count(*)::bigint,
       'Активные лицензии с истёкшим сроком'
FROM sam_licenses
WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at < current_date
UNION ALL
SELECT 'banned_products_installed',
       'high',
       count(*)::bigint,
       'Установки запрещённых продуктов'
FROM sam_installations i
JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed' AND p.status = 'banned'
UNION ALL
SELECT 'over_licensed_products',
       'high',
       count(*)::bigint,
       'Продукты, где установок больше, чем оплаченных мест'
FROM (
    SELECT p.id
    FROM sam_products p
    JOIN (
        SELECT product_id, sum(seats)::bigint AS seats
        FROM sam_licenses
        WHERE status = 'active' AND (expires_at IS NULL OR expires_at >= current_date)
        GROUP BY product_id
    ) licensed ON licensed.product_id = p.id
    WHERE (
        SELECT count(DISTINCT coalesce(nullif(i.device_name, ''), 'installation:' || i.id))
        FROM sam_installations i
        WHERE i.product_id = p.id AND i.status = 'installed'
    ) > licensed.seats
) overused
UNION ALL
SELECT 'inactive_products_installed',
       'medium',
       count(*)::bigint,
       'Установки продуктов, выведенных из активного статуса'
FROM sam_installations i
JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed' AND p.status IN ('inactive', 'archived')
UNION ALL
SELECT 'products_without_license',
       'medium',
       count(*)::bigint,
       'Активные продукты без действующей лицензии'
FROM sam_products p
WHERE p.status = 'active'
  AND NOT EXISTS (
      SELECT 1 FROM sam_licenses l
      WHERE l.product_id = p.id AND l.status = 'active'
        AND (l.expires_at IS NULL OR l.expires_at >= current_date)
  );

-- +goose Down
UPDATE sam_products SET status = 'inactive' WHERE status = 'banned';
ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'inactive', 'archived'));

CREATE OR REPLACE VIEW sam_risk_overview AS
SELECT 'unmatched_installations'::text AS risk,
       'high'::text AS severity,
       count(*)::bigint AS affected,
       'Установки без сопоставленного продукта'::text AS detail
FROM sam_installations
WHERE product_id IS NULL AND status = 'installed'
UNION ALL
SELECT 'expired_licenses',
       'high',
       count(*)::bigint,
       'Активные лицензии с истёкшим сроком'
FROM sam_licenses
WHERE status = 'active' AND expires_at IS NOT NULL AND expires_at < current_date
UNION ALL
SELECT 'over_licensed_products',
       'high',
       count(*)::bigint,
       'Продукты, где установок больше, чем оплаченных мест'
FROM (
    SELECT p.id
    FROM sam_products p
    JOIN (
        SELECT product_id, sum(seats)::bigint AS seats
        FROM sam_licenses
        WHERE status = 'active' AND (expires_at IS NULL OR expires_at >= current_date)
        GROUP BY product_id
    ) licensed ON licensed.product_id = p.id
    WHERE (
        SELECT count(*) FROM sam_installations i
        WHERE i.product_id = p.id AND i.status = 'installed'
    ) > licensed.seats
) overused
UNION ALL
SELECT 'inactive_products_installed',
       'medium',
       count(*)::bigint,
       'Установки продуктов, выведенных из активного статуса'
FROM sam_installations i
JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed' AND p.status <> 'active'
UNION ALL
SELECT 'products_without_license',
       'medium',
       count(*)::bigint,
       'Активные продукты без действующей лицензии'
FROM sam_products p
WHERE p.status = 'active'
  AND NOT EXISTS (
      SELECT 1 FROM sam_licenses l
      WHERE l.product_id = p.id AND l.status = 'active'
        AND (l.expires_at IS NULL OR l.expires_at >= current_date)
  );
