-- +goose Up
CREATE TABLE sam_ai_actions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint REFERENCES sam_ai_conversations(id) ON DELETE SET NULL,
    actor text NOT NULL,
    intent text NOT NULL CHECK (intent IN (
        'match_aliases',
        'set_status',
        'set_category',
        'update_license',
        'archive_product',
        'create_product'
    )),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    preview jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'confirmed', 'applied', 'cancelled', 'failed')),
    affected bigint NOT NULL DEFAULT 0,
    error text NOT NULL DEFAULT '',
    fingerprint text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    undone_at timestamptz,
    before jsonb,
    after jsonb
);
CREATE INDEX sam_ai_actions_actor_idx ON sam_ai_actions (actor, id DESC);
CREATE INDEX sam_ai_actions_conversation_idx ON sam_ai_actions (conversation_id, id DESC);
CREATE INDEX sam_ai_actions_status_idx ON sam_ai_actions (status, id DESC);
CREATE INDEX sam_ai_actions_applied_idx ON sam_ai_actions (applied_at DESC) WHERE status = 'applied';

-- +goose Down
DROP TABLE sam_ai_actions;
