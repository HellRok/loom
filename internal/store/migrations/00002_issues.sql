-- +goose Up
-- Issue numbers are allocated from this counter, so a deleted #N is never
-- reused.
ALTER TABLE projects ADD COLUMN next_issue INTEGER NOT NULL DEFAULT 1;

CREATE TABLE issues (
    id         INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    number     INTEGER NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('issue', 'map')),
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    state      TEXT NOT NULL CHECK (state IN ('open', 'closed')),
    author     TEXT NOT NULL,
    assignee   TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version    INTEGER NOT NULL,
    UNIQUE (project_id, number)
);

CREATE TABLE comments (
    id         INTEGER PRIMARY KEY,
    issue_id   INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    author     TEXT NOT NULL,
    session    TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX comments_issue ON comments (issue_id);

-- data is a JSON object of the Event's type-specific fields, e.g. the
-- previous value of an edited title or body.
CREATE TABLE events (
    id       INTEGER PRIMARY KEY,
    issue_id INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    at       TEXT NOT NULL,
    actor    TEXT NOT NULL,
    session  TEXT NOT NULL,
    type     TEXT NOT NULL,
    data     TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX events_issue ON events (issue_id);
