//go:build unix

package shim

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// stopTimeout caps how long Stop waits for the daemon to exit; it drains
// for about 5s.
const stopTimeout = 10 * time.Second

// spawnServe starts this binary's `serve` detached in its own session, with
// its output appended to serve.log, so nothing but MCP reaches our stdout.
func (s *Shim) spawnServe(context.Context) (<-chan error, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locating the loom binary: %w", err)
	}
	if err := os.MkdirAll(s.Home, 0o700); err != nil {
		return nil, fmt.Errorf("creating Loom home: %w", err)
	}
	log, err := os.OpenFile(s.LogPath(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening the daemon log: %w", err)
	}
	defer log.Close()
	// Not CommandContext: the daemon outlives this call and this shim.
	cmd := exec.Command(exe, "serve")
	cmd.Env = append(os.Environ(), "LOOM_HOME="+s.Home)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("running %s serve: %w", exe, err)
	}
	// Waiting (rather than releasing) reaps the daemon if it exits while
	// we're alive, and tells start a failed startup at once.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	return exited, nil
}

// Stop stops the daemon serving t.Home: the pid in its lockfile must be the
// pid /healthz reports (and expect, unless that is 0), which then gets
// SIGTERM. Stop returns once that process has exited, so its lock is free
// for a successor. It reports false, signalling nothing, if no such daemon
// runs.
func (t Target) Stop(ctx context.Context, expect int) (bool, error) {
	b, err := os.ReadFile(t.lockPath())
	if err != nil {
		return false, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || (expect != 0 && pid != expect) {
		return false, nil
	}
	h, err := t.probe(ctx)
	if err != nil || h.PID != pid {
		return false, nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return false, fmt.Errorf("signalling pid %d: %w", pid, err)
	}
	for deadline := time.Now().Add(stopTimeout); time.Now().Before(deadline); {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return true, nil
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
	return true, fmt.Errorf("pid %d still running %s after SIGTERM", pid, stopTimeout)
}
