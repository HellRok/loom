## Testing

`go test ./...` runs the unit and handler tests. **System tests** boot a real Loom daemon and drive the web UI in headless Chrome (chromedp, using the locally installed Chrome); they live in `internal/systest` behind the `system` build tag. Run them with `go test -tags system ./internal/systest/`.

## Agent skills

### Issue tracker

Issues are tracked in Loom, a local MCP issue tracker (installed by `loom init`, replacing the tracker chosen in `/setup-matt-pocock-skills`). See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical labels, used as-is (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
