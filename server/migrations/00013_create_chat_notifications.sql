-- +goose Up
CREATE TABLE chat_notifications (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES local_accounts(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('block_completed', 'merge_request_created')),
    dedup_key text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz
);

-- The unique pair is the deduplication authority: the same event key may be
-- replayed any number of times (planner progress is recomputed on every read),
-- but the row is created exactly once per user.
ALTER TABLE chat_notifications
    ADD CONSTRAINT chat_notifications_user_dedup_unique UNIQUE (user_id, dedup_key);

CREATE INDEX chat_notifications_user_recent_idx
    ON chat_notifications (user_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS chat_notifications_user_recent_idx;
ALTER TABLE chat_notifications DROP CONSTRAINT IF EXISTS chat_notifications_user_dedup_unique;
DROP TABLE IF EXISTS chat_notifications;
