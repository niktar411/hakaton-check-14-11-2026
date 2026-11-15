-- Редактирование данных SAM: прямые CRUD-операции для участников.
-- Права проверяет сервис; здесь только хранилище. Каждая запись попадает
-- в sam_audit_log (actor, action, before/after) со стороны сервиса.

-- name: SamEditProductByID :one
SELECT id, name, vendor, version, category, status,
       section_code, class_code, license_metric,
       registry_rf, registry_entry, is_trackable,
       created_at, updated_at
FROM sam_products
WHERE id = $1;

-- name: SamEditCreateProduct :one
INSERT INTO sam_products(name, vendor, version, category, status,
                         section_code, class_code, license_metric,
                         registry_rf, registry_entry, is_trackable)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, name, vendor, version, category, status,
          section_code, class_code, license_metric,
          registry_rf, registry_entry, is_trackable,
          created_at, updated_at;

-- name: SamEditUpdateProduct :one
UPDATE sam_products
SET name = $2, vendor = $3, version = $4, category = $5, status = $6,
    section_code = $7, class_code = $8, license_metric = $9,
    registry_rf = $10, registry_entry = $11, is_trackable = $12,
    updated_at = now()
WHERE id = $1
RETURNING id, name, vendor, version, category, status,
          section_code, class_code, license_metric,
          registry_rf, registry_entry, is_trackable,
          created_at, updated_at;

-- name: SamEditLicenseByID :one
SELECT id, product_id, license_type, seats, cost_per_seat, currency,
       starts_at, expires_at, contract
FROM sam_licenses
WHERE id = $1;

-- name: SamEditCreateLicense :one
INSERT INTO sam_licenses(product_id, license_type, seats, cost_per_seat,
                         currency, starts_at, expires_at, contract)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, product_id, license_type, seats, cost_per_seat, currency,
          starts_at, expires_at, contract;

-- name: SamEditUpdateLicense :one
UPDATE sam_licenses
SET product_id = $2, license_type = $3, seats = $4, cost_per_seat = $5,
    currency = $6, starts_at = $7, expires_at = $8, contract = $9,
    updated_at = now()
WHERE id = $1
RETURNING id, product_id, license_type, seats, cost_per_seat, currency,
          starts_at, expires_at, contract;

-- name: SamEditDeleteLicense :execrows
DELETE FROM sam_licenses WHERE id = $1;

-- name: SamEditInstallationByID :one
SELECT id, product_id, raw_name, device_name, user_name, version, source, status
FROM sam_installations
WHERE id = $1;

-- name: SamEditCreateInstallation :one
INSERT INTO sam_installations(raw_name, device_name, user_name, version,
                              product_id, source, status)
VALUES ($1, $2, $3, $4, $5, 'manual', $6)
RETURNING id, product_id, raw_name, device_name, user_name, version, source, status;

-- name: SamEditUpdateInstallation :one
UPDATE sam_installations
SET raw_name = $2, device_name = $3, user_name = $4, version = $5,
    product_id = $6, status = $7, last_seen_at = now()
WHERE id = $1
RETURNING id, product_id, raw_name, device_name, user_name, version, source, status;

-- name: SamEditDeleteInstallation :execrows
DELETE FROM sam_installations WHERE id = $1;

-- name: SamEditProductUsage :one
SELECT (SELECT count(*) FROM sam_licenses l WHERE l.product_id = $1)::bigint AS license_count,
       (SELECT count(*) FROM sam_installations i WHERE i.product_id = $1)::bigint AS installation_count;

-- name: SamEditLicenseView :one
SELECT l.id,
       p.name AS product_name,
       p.vendor AS product_vendor,
       l.license_type AS kind,
       l.seats::int AS seats_total,
       (SELECT count(DISTINCT coalesce(nullif(i.device_name, ''), 'installation:' || i.id))::int
        FROM sam_installations i
        WHERE i.product_id = p.id AND i.status = 'installed') AS seats_used,
       l.contract,
       l.expires_at AS valid_until,
       CASE
           WHEN l.status <> 'active' THEN 'expired'
           WHEN l.expires_at IS NULL THEN 'active'
           WHEN l.expires_at < current_date THEN 'expired'
           WHEN l.expires_at <= current_date + 30 THEN 'expiring'
           ELSE 'active'
       END::text AS status
FROM sam_licenses l
JOIN sam_products p ON p.id = l.product_id
WHERE l.id = $1;

-- name: SamEditInstallationView :one
SELECT i.id,
       i.product_id,
       p.name AS product_name,
       i.raw_name,
       i.device_name,
       i.user_name,
       i.version,
       i.source,
       i.status,
       i.last_seen_at
FROM sam_installations i
LEFT JOIN sam_products p ON p.id = i.product_id
WHERE i.id = $1;

-- name: SamEditAliasesByProduct :many
SELECT id, product_id, alias, normalized_alias, created_by, created_at
FROM sam_product_aliases
WHERE product_id = $1
ORDER BY id;

-- name: SamEditAuditInsert :exec
INSERT INTO sam_audit_log(actor, action, entity_type, entity_id, before, after)
VALUES ($1, $2, $3, $4, $5, $6);
