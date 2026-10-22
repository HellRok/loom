package shim

import "context"

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
