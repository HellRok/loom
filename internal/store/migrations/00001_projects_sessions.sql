-- +goose Up
CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    root       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    first_seen TEXT NOT NULL,
    last_seen  TEXT NOT NULL
);
