-- +goose Up
-- Composite indexes for the compliance hot path: license totals are joined
-- to installations per product and grouped by device, and every public read
-- filters licenses by product + status + expiry.
CREATE INDEX sam_installations_product_device_idx
    ON sam_installations (product_id, device_name);

CREATE INDEX sam_licenses_product_state_idx
    ON sam_licenses (product_id, status, expires_at);

CREATE INDEX sam_products_section_class_idx
    ON sam_products (section_code, class_code);

-- +goose Down
DROP INDEX IF EXISTS sam_products_section_class_idx;
DROP INDEX IF EXISTS sam_licenses_product_state_idx;
DROP INDEX IF EXISTS sam_installations_product_device_idx;
