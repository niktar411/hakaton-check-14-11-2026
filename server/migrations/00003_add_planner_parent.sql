-- +goose Up
ALTER TABLE planner_blocks
    ADD COLUMN parent_id bigint REFERENCES planner_blocks(id) ON DELETE SET NULL;
CREATE INDEX planner_blocks_parent_idx ON planner_blocks(parent_id);

-- +goose Down
DROP INDEX planner_blocks_parent_idx;
ALTER TABLE planner_blocks DROP COLUMN parent_id;
