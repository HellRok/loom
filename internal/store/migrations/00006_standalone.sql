-- +goose Up
-- A Standalone Project keeps its skill config in Loom rather than in the
-- repo; it is off until `loom init --standalone` sets it.
ALTER TABLE projects ADD COLUMN standalone INTEGER NOT NULL DEFAULT 0;
