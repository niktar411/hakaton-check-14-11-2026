-- +goose Up
ALTER TABLE sam_products
    ADD COLUMN status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive', 'archived', 'banned'));

CREATE INDEX sam_products_status_idx ON sam_products (status);

ALTER TABLE sam_licenses
    ADD COLUMN cost_per_seat bigint NOT NULL DEFAULT 0 CHECK (cost_per_seat >= 0),
    ADD COLUMN currency text NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB');

-- +goose Down
ALTER TABLE sam_licenses
    DROP COLUMN currency,
    DROP COLUMN cost_per_seat;

DROP INDEX sam_products_status_idx;

ALTER TABLE sam_products DROP COLUMN status;
