package shim

import (
	"context"
	"testing"
	"time"
)

// NewWithSpawn returns a shim whose daemon spawns go through spawn instead
// of starting a process. spawn returns a channel that yields the daemon's
// exit.
func NewWithSpawn(t Target, spawn func(context.Context) (<-chan error, error)) (*Shim, error) {
	s, err := New(t)
	if err != nil {
		return nil, err
	}
	s.spawn = spawn
	return s, nil
}

// SetInstructionsTimeout shortens how long a handshake waits for the
// daemon to name the Project, until the test ends.
func SetInstructionsTimeout(t testing.TB, d time.Duration) {
	old := instructionsTimeout
	instructionsTimeout = d
	t.Cleanup(func() { instructionsTimeout = old })
}
