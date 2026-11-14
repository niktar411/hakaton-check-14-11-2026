-- +goose Up
CREATE TABLE sam_products (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    vendor text NOT NULL CHECK (length(btrim(vendor)) > 0),
    category text NOT NULL CHECK (length(btrim(category)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sam_products_name_vendor_idx ON sam_products (lower(name), lower(vendor));

CREATE TABLE sam_assets (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hostname text NOT NULL UNIQUE CHECK (length(btrim(hostname)) > 0),
    platform text NOT NULL CHECK (length(btrim(platform)) > 0),
    owner text NOT NULL DEFAULT '',
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_assets_last_seen_idx ON sam_assets (last_seen_at DESC NULLS LAST);

CREATE TABLE sam_licenses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id bigint NOT NULL REFERENCES sam_products(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('subscription', 'perpetual', 'trial')),
    seats_total integer NOT NULL CHECK (seats_total >= 0),
    seats_used integer NOT NULL DEFAULT 0 CHECK (seats_used >= 0),
    valid_until date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (seats_used <= seats_total)
);
CREATE INDEX sam_licenses_product_idx ON sam_licenses (product_id);
CREATE INDEX sam_licenses_valid_until_idx ON sam_licenses (valid_until);

CREATE TABLE sam_installations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    asset_id bigint NOT NULL REFERENCES sam_assets(id) ON DELETE CASCADE,
    product_id bigint REFERENCES sam_products(id) ON DELETE SET NULL,
    detected_name text NOT NULL CHECK (length(btrim(detected_name)) > 0),
    version text NOT NULL DEFAULT '',
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_installations_asset_idx ON sam_installations (asset_id);
CREATE INDEX sam_installations_product_idx ON sam_installations (product_id);

CREATE TABLE sam_findings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id bigint REFERENCES sam_products(id) ON DELETE SET NULL,
    asset_id bigint REFERENCES sam_assets(id) ON DELETE SET NULL,
    code text NOT NULL CHECK (length(btrim(code)) > 0),
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    status text NOT NULL CHECK (status IN ('open', 'acknowledged', 'resolved')),
    details text NOT NULL DEFAULT '',
    detected_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);
CREATE INDEX sam_findings_status_idx ON sam_findings (status, detected_at DESC);
CREATE INDEX sam_findings_severity_idx ON sam_findings (severity);

CREATE TABLE sam_matches (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    installation_id bigint NOT NULL REFERENCES sam_installations(id) ON DELETE CASCADE,
    product_id bigint REFERENCES sam_products(id) ON DELETE SET NULL,
    confidence double precision NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    status text NOT NULL CHECK (status IN ('suggested', 'confirmed', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz
);
CREATE INDEX sam_matches_status_idx ON sam_matches (status, created_at DESC);

-- +goose Down
DROP TABLE sam_matches;
DROP TABLE sam_findings;
DROP TABLE sam_installations;
DROP TABLE sam_licenses;
DROP TABLE sam_assets;
DROP TABLE sam_products;
