package shim

import "github.com/google/uuid"

// CLISessionPrefix starts the Session id of a `loom` subcommand talking to
// the daemon. The daemon doesn't record these as agent Sessions.
const CLISessionPrefix = "loom-cli-"

// AsCLI makes s a CLI Session, so the daemon doesn't count it as a
// connected agent.
func (s *Shim) AsCLI() { s.session = CLISessionPrefix + uuid.NewString() }
