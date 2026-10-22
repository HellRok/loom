# Background daemon spawning and single-instance in Go

Type: research
Status: resolved
Blocked by: 

## Question

How should `loom mcp` detect a running daemon and, if absent, start `loom serve` detached in the background on macOS and Linux? Cover: detaching from the parent (setsid / process group, stdio redirection so the stdio shim isn't polluted), single-instance guarantees when two `loom mcp` processes race at startup (lockfile, pidfile, port bind, flock), health checks and readiness waiting, and stale pid/lock recovery. Surface the idiomatic, well-trodden approaches with primary sources.

## Answer

Findings: branch `research/daemon-spawning`, file `research/daemon-spawning.md`.

- **Detect**: `loom mcp` probes `GET 127.0.0.1:<port>/healthz` (~1s timeout); response identifies Loom + version so a foreign process on the port isn't mistaken for it.
- **Spawn**: `os.Executable()` + `serve`, `SysProcAttr{Setsid: true}`, stdin nil, stdout/stderr to a log file (never the shim's stdout, which is the MCP channel), `Start()` + `Process.Release()`. Same pattern as gopls.
- **Single instance**: daemon holds `flock(LOCK_EX|LOCK_NB)` on a lockfile for its lifetime; the 127.0.0.1 port bind is a second guard. Racing spawns: loser fails lock/bind and exits quietly. Kernel releases on death, so no stale-lock recovery.
- **Readiness**: daemon locks, opens DB, then binds; client polls `/healthz` with exponential backoff from 50ms, capped ~5–10s.
- **No pidfile**: check-then-write is racy and pids are reused; optional advisory pid/port/version file only.
- **Gotchas**: keep the lock `*os.File` reachable (GC close drops the lock); `/healthz` version enables detecting a stale daemon after upgrade (feeds the upgrade fog). Verified on macOS; Linux from docs.
