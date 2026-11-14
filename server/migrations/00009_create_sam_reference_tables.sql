-- +goose Up
CREATE TABLE sam_vendors (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    normalized_name text NOT NULL UNIQUE,
    country text NOT NULL DEFAULT '',
    is_russian boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sam_employees (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    employee_code text NOT NULL UNIQUE,
    display_name text NOT NULL DEFAULT '',
    department text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sam_devices (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_code text NOT NULL UNIQUE,
    platform text NOT NULL DEFAULT '',
    employee_id bigint REFERENCES sam_employees(id) ON DELETE SET NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_devices_employee_idx ON sam_devices(employee_id);

CREATE TABLE sam_imports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source text NOT NULL DEFAULT 'csv' CHECK (source IN ('csv', 'agent')),
    file_name text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    rows_total bigint NOT NULL DEFAULT 0 CHECK (rows_total >= 0),
    rows_inserted bigint NOT NULL DEFAULT 0 CHECK (rows_inserted >= 0),
    rows_updated bigint NOT NULL DEFAULT 0 CHECK (rows_updated >= 0),
    rows_skipped bigint NOT NULL DEFAULT 0 CHECK (rows_skipped >= 0),
    rows_unmatched bigint NOT NULL DEFAULT 0 CHECK (rows_unmatched >= 0),
    error text NOT NULL DEFAULT '',
    started_by text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX sam_imports_status_idx ON sam_imports(status, id DESC);

CREATE TABLE sam_audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor text NOT NULL,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL DEFAULT '',
    before jsonb,
    after jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_audit_log_actor_idx ON sam_audit_log(actor, id DESC);
CREATE INDEX sam_audit_log_entity_idx ON sam_audit_log(entity_type, entity_id, id DESC);
CREATE INDEX sam_audit_log_created_idx ON sam_audit_log(created_at DESC);

ALTER TABLE sam_products
    ADD COLUMN vendor_id bigint REFERENCES sam_vendors(id) ON DELETE SET NULL,
    ADD COLUMN section_code text NOT NULL DEFAULT '',
    ADD COLUMN class_code text NOT NULL DEFAULT '',
    ADD COLUMN license_metric text NOT NULL DEFAULT 'device'
        CHECK (license_metric IN ('device', 'user', 'installation', 'none')),
    ADD COLUMN registry_rf boolean NOT NULL DEFAULT false,
    ADD COLUMN registry_entry text NOT NULL DEFAULT '',
    ADD COLUMN is_trackable boolean NOT NULL DEFAULT true;
CREATE INDEX sam_products_vendor_idx ON sam_products(vendor_id);
CREATE INDEX sam_products_registry_idx ON sam_products(registry_rf) WHERE registry_rf;

ALTER TABLE sam_licenses
    ADD COLUMN contract text NOT NULL DEFAULT '';

ALTER TABLE sam_installations
    ADD COLUMN device_id bigint REFERENCES sam_devices(id) ON DELETE SET NULL,
    ADD COLUMN employee_id bigint REFERENCES sam_employees(id) ON DELETE SET NULL;
CREATE INDEX sam_installations_device_idx ON sam_installations(device_id);
CREATE INDEX sam_installations_employee_idx ON sam_installations(employee_id);

ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'outdated', 'inactive', 'banned', 'archived'));

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
SELECT 'outdated_products_installed',
       'medium',
       count(*)::bigint,
       'Установки продуктов, выведенных из активного статуса'
FROM sam_installations i
JOIN sam_products p ON p.id = i.product_id
WHERE i.status = 'installed' AND p.status IN ('outdated', 'archived')
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

ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
UPDATE sam_products SET status = 'inactive' WHERE status = 'outdated';
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'inactive', 'archived', 'banned'));

ALTER TABLE sam_installations DROP COLUMN IF EXISTS employee_id;
ALTER TABLE sam_installations DROP COLUMN IF EXISTS device_id;
ALTER TABLE sam_licenses DROP COLUMN IF EXISTS contract;
ALTER TABLE sam_products DROP COLUMN IF EXISTS is_trackable;
ALTER TABLE sam_products DROP COLUMN IF EXISTS registry_entry;
ALTER TABLE sam_products DROP COLUMN IF EXISTS registry_rf;
ALTER TABLE sam_products DROP COLUMN IF EXISTS license_metric;
ALTER TABLE sam_products DROP COLUMN IF EXISTS class_code;
ALTER TABLE sam_products DROP COLUMN IF EXISTS section_code;
ALTER TABLE sam_products DROP COLUMN IF EXISTS vendor_id;

DROP TABLE sam_audit_log;
DROP TABLE sam_imports;
DROP TABLE sam_devices;
DROP TABLE sam_employees;
DROP TABLE sam_vendors;
