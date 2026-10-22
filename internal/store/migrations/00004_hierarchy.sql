-- +goose Up
-- An Issue has at most one parent in its own Project; deleting the parent
-- leaves its children parentless.
ALTER TABLE issues ADD COLUMN parent_id INTEGER REFERENCES issues (id) ON DELETE SET NULL;
CREATE INDEX issues_parent ON issues (parent_id);

-- issue_id is blocked by blocker_id. Edges go with either Issue.
CREATE TABLE issue_blockers (
    issue_id   INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    blocker_id INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, blocker_id),
    CHECK (issue_id <> blocker_id)
);
CREATE INDEX issue_blockers_blocker ON issue_blockers (blocker_id);
