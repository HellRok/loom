-- +goose Up
-- A Claim sets the assignee and also records which Session holds the Issue
-- and when it took it; both are null while the Issue is unclaimed.
ALTER TABLE issues ADD COLUMN holder_session TEXT;
ALTER TABLE issues ADD COLUMN claimed_at TEXT;
CREATE INDEX issues_holder ON issues (holder_session);
