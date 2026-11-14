-- +goose Up
ALTER TABLE planner_blocks
    ADD COLUMN completed_at timestamptz;

CREATE TABLE chat_change_prompts (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username   text        NOT NULL REFERENCES local_accounts(username) ON DELETE CASCADE,
    block_id   bigint      NOT NULL REFERENCES planner_blocks(id) ON DELETE CASCADE,
    mr_url     text        NOT NULL DEFAULT '',
    pulled_at  timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (block_id, username)
);

CREATE INDEX chat_change_prompts_pending_idx
    ON chat_change_prompts(username, created_at)
    WHERE pulled_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS chat_change_prompts_pending_idx;
DROP TABLE IF EXISTS chat_change_prompts;
ALTER TABLE planner_blocks DROP COLUMN IF EXISTS completed_at;
