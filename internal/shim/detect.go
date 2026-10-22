// Package shim implements `loom mcp` and `loom stop`: it finds, starts and
// stops the daemon for a Loom home, and proxies an agent's stdio MCP session
// to it.
package shim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/mod/semver"
)

// probeTimeout caps one /healthz request.
const probeTimeout = time.Second

// Target is the daemon this shim wants: one serving Home on Port, at least
// as new as Version.
type Target struct {
	Home    string
	Port    int
	Version string
}

// Health is what a daemon reports at /healthz.
type Health struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Home    string `json:"home"`
}

// Action is what to do about the daemon found on the port.
type Action int

const (
	// Use the running daemon as is.
	Use Action = iota
	// Spawn a daemon: nothing is listening.
	Spawn
	// Replace an older daemon: stop it, then spawn.
	Replace
)

func (a Action) String() string {
	return [...]string{"Use", "Spawn", "Replace"}[a]
}

// errNoDaemon means nothing accepted a connection on the port.
var errNoDaemon = errors.New("no daemon listening")

// Detect probes the port and decides what to do about whatever answers. A
// daemon for another Loom home, or something that isn't Loom, is an error.
func (t Target) Detect(ctx context.Context) (Action, Health, error) {
	h, err := t.probe(ctx)
	if errors.Is(err, errNoDaemon) {
		return Spawn, Health{}, nil
	}
	if err != nil {
		return 0, Health{}, err
	}
	if filepath.Clean(h.Home) != filepath.Clean(t.Home) {
		return 0, h, fmt.Errorf("the Loom daemon on port %d serves the Loom home %s, not %s; to run both, set a different `port` in %s",
			t.Port, h.Home, t.Home, t.configPath())
	}
	if replaces(t.Version, h.Version) {
		return Replace, h, nil
	}
	return Use, h, nil
}

// probe asks the port's /healthz who is there.
func (t Target) probe(ctx context.Context) (Health, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", t.baseURL()+"/healthz", nil)
	if err != nil {
		return Health{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if errors.Is(err, syscall.ECONNREFUSED) {
		return Health{}, errNoDaemon
	}
	if err != nil {
		return Health{}, fmt.Errorf("no answer from /healthz on port %d: %w; if a program other than Loom uses that port, set a different `port` in %s", t.Port, err, t.configPath())
	}
	defer resp.Body.Close()
	var h Health
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&h) != nil || h.Name != "loom" {
		return Health{}, fmt.Errorf("something other than Loom is listening on port %d; set a different `port` in %s", t.Port, t.configPath())
	}
	return h, nil
}

// replaces reports whether a shim at version self replaces a daemon at
// version daemon. Releases compare by semver, so only an older daemon is
// replaced; a dev build matches only the identical build string, so any
// other build replaces it, and it replaces any other build.
func replaces(self, daemon string) bool {
	if semver.IsValid(daemon) && semver.IsValid(self) {
		return semver.Compare(daemon, self) < 0
	}
	return daemon != self
}

func (t Target) baseURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(t.Port)
}

func (t Target) configPath() string {
	return filepath.Join(t.Home, "config.toml")
}

func (t Target) lockPath() string {
	return filepath.Join(t.Home, "loom.lock")
}

// LogPath is the daemon's log, where spawned daemons write.
func (t Target) LogPath() string {
	return filepath.Join(t.Home, "serve.log")
}
