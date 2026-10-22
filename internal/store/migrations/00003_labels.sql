-- +goose Up
CREATE TABLE labels (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    colour      TEXT NOT NULL,
    UNIQUE (project_id, name)
);

CREATE TABLE issue_labels (
    issue_id INTEGER NOT NULL REFERENCES issues (id) ON DELETE CASCADE,
    label_id INTEGER NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    PRIMARY KEY (issue_id, label_id)
);
CREATE INDEX issue_labels_label ON issue_labels (label_id);

-- Seed Projects that predate Labels with the same default set, in the same
-- order and colours, as a new Project gets (see defaultLabels in labels.go).
INSERT INTO labels (project_id, name, colour)
SELECT p.id, d.name, d.colour
FROM projects p
CROSS JOIN (
    SELECT 'bug' AS name, 'red' AS colour UNION ALL
    SELECT 'enhancement', 'orange' UNION ALL
    SELECT 'needs-triage', 'yellow' UNION ALL
    SELECT 'needs-info', 'green' UNION ALL
    SELECT 'ready-for-agent', 'teal' UNION ALL
    SELECT 'ready-for-human', 'blue' UNION ALL
    SELECT 'wontfix', 'purple' UNION ALL
    SELECT 'wayfinder:research', 'pink' UNION ALL
    SELECT 'wayfinder:prototype', 'red' UNION ALL
    SELECT 'wayfinder:grilling', 'orange' UNION ALL
    SELECT 'wayfinder:task', 'yellow'
) d;
