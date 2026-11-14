-- +goose Up
INSERT INTO sam_vendors (name, normalized_name, is_russian)
SELECT DISTINCT NULLIF(btrim(vendor), ''), lower(NULLIF(btrim(vendor), '')), false
FROM sam_products
WHERE NULLIF(btrim(vendor), '') IS NOT NULL
ON CONFLICT (normalized_name) DO NOTHING;

UPDATE sam_products p
SET vendor_id = v.id
FROM sam_vendors v
WHERE p.vendor_id IS NULL
  AND lower(NULLIF(btrim(p.vendor), '')) = v.normalized_name;

INSERT INTO sam_employees (employee_code)
SELECT DISTINCT 'EMP-' || upper(substr(md5(NULLIF(btrim(user_name), '')), 1, 12))
FROM sam_installations
WHERE NULLIF(btrim(user_name), '') IS NOT NULL
ON CONFLICT (employee_code) DO NOTHING;

UPDATE sam_installations i
SET employee_id = e.id
FROM sam_employees e
WHERE i.employee_id IS NULL
  AND e.employee_code = 'EMP-' || upper(substr(md5(NULLIF(btrim(i.user_name), '')), 1, 12));

INSERT INTO sam_devices (device_code, last_seen_at)
SELECT NULLIF(btrim(device_name), ''), max(last_seen_at)
FROM sam_installations
WHERE NULLIF(btrim(device_name), '') IS NOT NULL
GROUP BY NULLIF(btrim(device_name), '')
ON CONFLICT (device_code) DO NOTHING;

UPDATE sam_installations i
SET device_id = d.id
FROM sam_devices d
WHERE i.device_id IS NULL
  AND NULLIF(btrim(i.device_name), '') = d.device_code;

UPDATE sam_devices d
SET employee_id = usage.employee_id
FROM (
    SELECT i.device_id,
           min(i.employee_id) AS employee_id,
           count(DISTINCT i.employee_id) AS owners
    FROM sam_installations i
    WHERE i.device_id IS NOT NULL AND i.employee_id IS NOT NULL
    GROUP BY i.device_id
) usage
WHERE usage.device_id = d.id
  AND usage.owners = 1
  AND d.employee_id IS NULL;

UPDATE sam_products SET status = 'outdated' WHERE status = 'inactive';

ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'outdated', 'banned', 'archived'));

-- +goose Down
ALTER TABLE sam_products DROP CONSTRAINT IF EXISTS sam_products_status_check;
UPDATE sam_products SET status = 'inactive' WHERE status = 'outdated';
ALTER TABLE sam_products ADD CONSTRAINT sam_products_status_check
    CHECK (status IN ('active', 'outdated', 'inactive', 'banned', 'archived'));

UPDATE sam_devices SET employee_id = NULL;
UPDATE sam_installations SET employee_id = NULL, device_id = NULL;
UPDATE sam_products SET vendor_id = NULL;
