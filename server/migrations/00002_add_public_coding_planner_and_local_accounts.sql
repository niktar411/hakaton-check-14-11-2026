-- +goose Up
CREATE TABLE local_accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (username ~ '^[a-z][a-z0-9_-]{2,31}$'),
    display_name text NOT NULL,
    password_hash text NOT NULL DEFAULT '',
    role text NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    enabled boolean NOT NULL DEFAULT true,
    color text NOT NULL CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    personal_context text NOT NULL DEFAULT '',
    tone text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE sessions ALTER COLUMN access_token DROP NOT NULL;
ALTER TABLE sessions ADD COLUMN account_id bigint REFERENCES local_accounts(id) ON DELETE SET NULL;
CREATE INDEX sessions_account_idx ON sessions(account_id);
CREATE TABLE application_settings (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE faq_entries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    question text NOT NULL,
    answer text NOT NULL,
    links jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX faq_entries_created_idx ON faq_entries(created_at DESC);
CREATE TABLE planner_blocks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    owner_id bigint NOT NULL REFERENCES local_accounts(id) ON DELETE RESTRICT,
    branch text NOT NULL DEFAULT '',
    merge_request_iid bigint,
    position integer NOT NULL CHECK (position >= 0),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX planner_blocks_owner_idx ON planner_blocks(owner_id);
CREATE UNIQUE INDEX planner_blocks_position_idx ON planner_blocks(position);
CREATE TABLE coding_tasks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES local_accounts(id) ON DELETE RESTRICT,
    parent_id bigint REFERENCES coding_tasks(id) ON DELETE SET NULL,
    request_text text NOT NULL,
    phase text NOT NULL CHECK (phase IN ('clarifying', 'awaiting_confirmation', 'running', 'awaiting_merge', 'completed', 'failed')),
    clarification text NOT NULL DEFAULT '',
    branch text NOT NULL DEFAULT '',
    merge_request_iid bigint,
    merge_request_url text NOT NULL DEFAULT '',
    failure_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX coding_tasks_account_idx ON coding_tasks(account_id, id DESC);
CREATE INDEX coding_tasks_parent_idx ON coding_tasks(parent_id);
CREATE TABLE coding_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES coding_tasks(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content text NOT NULL,
    tool_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX coding_messages_task_idx ON coding_messages(task_id, id);
CREATE TABLE merge_approvals (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merge_request_iid bigint NOT NULL,
    source_branch text NOT NULL,
    target_branch text NOT NULL,
    commit_sha text NOT NULL,
    approved_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merge_request_iid, commit_sha)
);
CREATE TABLE deployment_operations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor text NOT NULL,
    action text NOT NULL CHECK (action IN ('start', 'stop', 'restart', 'deploy', 'rollback', 'create_test_database', 'drop_test_database')),
    target text NOT NULL,
    release text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'rolled_back')),
    backup_name text NOT NULL DEFAULT '',
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX deployment_operations_created_idx ON deployment_operations(created_at DESC);
-- +goose Down
DROP TABLE deployment_operations;
DROP TABLE merge_approvals;
DROP TABLE coding_messages;
DROP TABLE coding_tasks;
DROP TABLE planner_blocks;
DROP TABLE faq_entries;
DROP TABLE application_settings;
DELETE FROM sessions WHERE access_token IS NULL;
ALTER TABLE sessions DROP COLUMN account_id;
ALTER TABLE sessions ALTER COLUMN access_token SET NOT NULL;
DROP TABLE local_accounts;
