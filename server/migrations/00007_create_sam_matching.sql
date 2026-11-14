-- +goose Up
ALTER TABLE sam_product_aliases
    ADD COLUMN confidence real NOT NULL DEFAULT 1.0 CHECK (confidence >= 0 AND confidence <= 1);

CREATE TABLE sam_match_candidates (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    installation_id bigint NOT NULL REFERENCES sam_installations(id) ON DELETE CASCADE,
    product_id bigint NOT NULL REFERENCES sam_products(id) ON DELETE CASCADE,
    raw_name text NOT NULL,
    score real NOT NULL CHECK (score >= 0 AND score <= 1),
    method text NOT NULL CHECK (method IN ('exact', 'token', 'fuzzy', 'manual')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'confirmed', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    resolved_by text,
    UNIQUE (installation_id, product_id)
);
CREATE INDEX sam_match_candidates_status_idx ON sam_match_candidates (status, score DESC, id);
CREATE INDEX sam_match_candidates_installation_idx ON sam_match_candidates (installation_id);
CREATE INDEX sam_match_candidates_product_idx ON sam_match_candidates (product_id);

-- +goose Down
DROP TABLE sam_match_candidates;
ALTER TABLE sam_product_aliases DROP COLUMN confidence;
