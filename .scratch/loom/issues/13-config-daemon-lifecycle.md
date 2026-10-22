# Config and daemon lifecycle

Type: grilling
Status: resolved
Assignee: Sean Earle
Blocked by: 

## Question

Where does Loom's config live and what's in it: file location, port selection (fixed default? override?), DB path, the configured assignee name, and how each is set (file, env, flag; does `loom init` or first run write any of it)? And how does the daemon's life end: idle timeout or run forever, is there a `loom stop`, and what happens when the shim's `/healthz` probe finds a daemon reporting an older version after `go install` of a newer `loom` (restart it, refuse, warn)?

## Answer

Resolved by grilling (2026-10-22).

- **Loom home**: `$LOOM_HOME`, default `~/.loom`; holds `config.toml`, `loom.db`, `loom.lock`, `serve.log`.
- **Config**: optional TOML, every key defaulted; no Loom command writes it. Daemon logs its effective config at startup. Changes take effect after a restart (`loom stop`; next `loom mcp` respawns).
- **Port**: fixed default `7337`, `port` override; never auto-picked. Shim and daemon read the same config. A non-Loom process on the port → error naming the port and the config key.
- **Assignee**: `assignee` key, default `git config --global user.name`, then `$USER`, resolved at daemon start. Existing assignees are never rewritten; filters match exactly.
- **Lifetime**: daemon runs until stopped; no idle timeout.
- **Stop**: daemon writes its pid into `loom.lock` after taking the flock; `/healthz` returns pid, version, and Loom home. `loom stop` reads the pid, confirms `/healthz` reports the same pid, sends SIGTERM, waits for the port to free; otherwise reports `not running` and signals nothing. On SIGTERM/SIGINT the daemon drains in-flight requests (~5s cap), checkpoints WAL, closes the DB, exits. No HTTP shutdown endpoint.
- **Security**: daemon rejects any request whose `Host` is not `127.0.0.1:<port>` or `localhost:<port>` (DNS-rebinding guard). No Origin rule needed.
- **Upgrade**: shim finds daemon older → stops it as `loom stop` does, spawns its own; in-flight calls on the old daemon surface as "outcome unknown". Daemon newer → use it (shim proxies live `tools/list`). Dev builds compare the full build string; any difference counts as older.
- **Home mismatch**: if `/healthz` reports a different Loom home than the shim's, the shim refuses, naming both paths and suggesting `port` in that home's config.
- **Supersedes** [Background daemon spawning and single-instance in Go](01-daemon-spawning.md)'s "no pidfile": the pid lives in the lockfile, trusted only after the `/healthz` pid check.
- Glossary: added **Loom home**.
