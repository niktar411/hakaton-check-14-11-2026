-- +goose Up
CREATE TABLE sessions (
    id text PRIMARY KEY, username text NOT NULL, access_token text NOT NULL,
    csrf_token text NOT NULL, expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE oauth_states (
    id text PRIMARY KEY, verifier text NOT NULL, expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE conversations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, username text NOT NULL,
    phase text NOT NULL CHECK (phase IN ('clarifying', 'ready')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_username_idx ON conversations(username);
CREATE TABLE messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id bigint NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant')), content text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_conversation_idx ON messages(conversation_id, id);
CREATE TABLE usage_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, request_id text NOT NULL UNIQUE,
    conversation_id bigint REFERENCES conversations(id) ON DELETE SET NULL,
    username text NOT NULL, model text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'completed', 'failed', 'usage_missing')),
    input_tokens bigint, output_tokens bigint, total_tokens bigint,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX usage_events_created_idx ON usage_events(created_at DESC);
CREATE TABLE audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, username text NOT NULL,
    action text NOT NULL, target text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE audit_events;
DROP TABLE usage_events;
DROP TABLE messages;
DROP TABLE conversations;
DROP TABLE oauth_states;
DROP TABLE sessions;
