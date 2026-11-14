-- +goose Up
CREATE TABLE sam_products (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    vendor text NOT NULL DEFAULT '',
    version text NOT NULL DEFAULT '',
    category text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sam_products_name_ci_idx ON sam_products (lower(name));
CREATE INDEX sam_products_status_idx ON sam_products (status);

CREATE TABLE sam_installations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id bigint REFERENCES sam_products(id) ON DELETE SET NULL,
    raw_name text NOT NULL,
    device_name text NOT NULL DEFAULT '',
    user_name text NOT NULL DEFAULT '',
    version text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'manual',
    status text NOT NULL DEFAULT 'installed' CHECK (status IN ('installed', 'removed', 'unknown')),
    installed_at timestamptz,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_installations_product_idx ON sam_installations (product_id);
CREATE INDEX sam_installations_unmatched_idx ON sam_installations (lower(raw_name)) WHERE product_id IS NULL;
CREATE INDEX sam_installations_status_idx ON sam_installations (status);

CREATE TABLE sam_product_aliases (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id bigint NOT NULL REFERENCES sam_products(id) ON DELETE CASCADE,
    alias text NOT NULL,
    normalized_alias text NOT NULL UNIQUE,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_product_aliases_product_idx ON sam_product_aliases (product_id);

CREATE TABLE sam_licenses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id bigint NOT NULL REFERENCES sam_products(id) ON DELETE CASCADE,
    license_type text NOT NULL DEFAULT 'subscription' CHECK (license_type IN ('subscription', 'perpetual', 'open_source', 'trial')),
    seats bigint NOT NULL DEFAULT 0 CHECK (seats >= 0),
    cost_per_seat bigint NOT NULL DEFAULT 0 CHECK (cost_per_seat >= 0),
    currency text NOT NULL DEFAULT 'RUB',
    starts_at date,
    expires_at date,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'expired', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_licenses_product_idx ON sam_licenses (product_id);
CREATE INDEX sam_licenses_expiry_idx ON sam_licenses (expires_at);
CREATE INDEX sam_licenses_status_idx ON sam_licenses (status);

CREATE VIEW sam_risk_overview AS
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

CREATE TABLE sam_ai_conversations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor text NOT NULL,
    mode text NOT NULL DEFAULT 'assistant' CHECK (mode IN ('assistant', 'operator')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_ai_conversations_actor_idx ON sam_ai_conversations (actor, mode, updated_at DESC);

CREATE TABLE sam_ai_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint NOT NULL REFERENCES sam_ai_conversations(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content text NOT NULL,
    tool_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sam_ai_messages_conversation_idx ON sam_ai_messages (conversation_id, id);

-- +goose Down
DROP TABLE sam_ai_messages;
DROP TABLE sam_ai_conversations;
DROP VIEW sam_risk_overview;
DROP TABLE sam_licenses;
DROP TABLE sam_product_aliases;
DROP TABLE sam_installations;
DROP TABLE sam_products;
