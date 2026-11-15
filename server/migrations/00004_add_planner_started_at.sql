-- +goose Up
ALTER TABLE planner_blocks
    ADD COLUMN started_at timestamptz;

-- +goose Down
ALTER TABLE planner_blocks DROP COLUMN started_at;
