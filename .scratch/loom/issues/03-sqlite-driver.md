# Go SQLite driver choice

Type: research
Status: resolved
Blocked by: 

## Question

Which Go SQLite driver fits a single static binary built with `go install`: `modernc.org/sqlite` (pure Go), `ncruces/go-sqlite3` (wasm), or `mattn/go-sqlite3` (cgo)? Compare: cgo requirement and cross-compilation, WAL mode, FTS5, JSON functions, performance, maturity, `database/sql` compatibility, and common migration tooling that works with it.

## Answer

Findings: branch `research/sqlite-driver`, file `research/sqlite-driver.md`.

- **Recommend** `modernc.org/sqlite` v1.61.0 (2026-10-21, SQLite 3.54.1) with `pressly/goose/v3` v3.29.0, migrations embedded via `embed.FS`.
- **No cgo**: `go install` and cross-compiling just work. `mattn/go-sqlite3` builds without cgo but fails at runtime on `Open`; cgo is off by default without a C toolchain or when cross-compiling, so it's ruled out.
- **Features**: WAL, JSON and FTS5 verified working in modernc with no build tags (ncruces and mattn need FTS5 enabled explicitly).
- **Performance**: ~1.3–2x C SQLite CPU on heavy queries; irrelevant at Loom's scale.
- **Runner-up**: `ncruces/go-sqlite3` v0.35.6: cgo-free but pre-1.0, slower, more memory, self-described less battle-tested VFS.
- **Gotchas**: set `busy_timeout`, `journal_mode(WAL)`, `foreign_keys(1)` in the DSN; `_txlock=immediate` for the concurrency-safe claim; `SetMaxOpenConns`; pin `modernc.org/libc` to modernc's required version; keep mattn out of the dependency graph.
