-- +goose Up
-- A Project's Glossary is one Markdown document with an optimistic version.
-- A Project has no row until its Glossary is first written; the row goes
-- with the Project.
CREATE TABLE glossaries (
    project_id INTEGER PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    version    INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
