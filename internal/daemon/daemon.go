// Package daemon runs `loom serve`: it owns the Loom home's lock and
// database and serves /mcp and /healthz on 127.0.0.1 (or the `listen` address).
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/HellRok/loom/internal/config"
	"github.com/HellRok/loom/internal/store"
	"github.com/HellRok/loom/internal/tools"
	"github.com/HellRok/loom/internal/web"
)

var (
	// ErrAlreadyRunning means another daemon holds this Loom home's lock.
	ErrAlreadyRunning = errors.New("another loom daemon holds the lock")
	// ErrPortInUse means another process is already listening on the port.
	ErrPortInUse = errors.New("port in use")
)

// drainTimeout caps how long shutdown waits for in-flight requests.
const drainTimeout = 5 * time.Second

// Options configure a daemon.
type Options struct {
	Config  config.Config
	Version string
	Logger  *slog.Logger
}

func (o Options) logger() *slog.Logger {
	if o.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return o.Logger
}

// Run starts the daemon and serves until ctx is cancelled. Startup order:
// take the lock, open and migrate the database, then bind.
func Run(ctx context.Context, o Options) error {
	home := o.Config.Home
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("creating Loom home: %w", err)
	}
	lockPath := filepath.Join(home, "loom.lock")
	lock, err := acquireLock(lockPath)
	if err != nil {
		return fmt.Errorf("locking %s: %w", lockPath, err)
	}
	defer lock.Close() // also releases the lock; lock must stay reachable until then

	st, err := store.Open(ctx, filepath.Join(home, "loom.db"))
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			o.logger().Error("closing database", "err", err)
		}
	}()

	addr := net.JoinHostPort(o.Config.ListenAddr(), strconv.Itoa(o.Config.Port))
	ln, err := net.Listen("tcp", addr)
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("%w: %w", ErrPortInUse, err)
	}
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	handler, closeStreams := newHandler(o, st)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	// Shutdown waits for open requests, and an event stream never ends on
	// its own.
	srv.RegisterOnShutdown(closeStreams)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	o.logger().Info("listening", "addr", addr)
	if !o.Config.Loopback() {
		o.logger().Warn("listening beyond this machine: Loom has no authentication, so anyone who can reach the port can read and change every Issue", "addr", addr)
	}

	select {
	case err := <-served:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}
	o.logger().Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	// Hitting the drain cap is expected, not a failure: in-flight requests
	// are cut off and shutdown carries on to checkpoint the WAL.
	if err := srv.Shutdown(sctx); err != nil {
		o.logger().Warn("drain timed out", "err", err)
		if err := srv.Close(); err != nil {
			o.logger().Error("closing server", "err", err)
		}
	}
	return nil
}

// acquireLock takes an exclusive, non-blocking lock on path and writes our
// pid into it.
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(f); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Handler is the daemon's HTTP surface, behind the Host check.
func Handler(o Options, st *store.Store) http.Handler {
	h, _ := newHandler(o, st)
	return h
}

// newHandler is Handler, with the function that ends the open event streams.
func newHandler(o Options, st *store.Store) (http.Handler, func()) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"name":    "loom",
			"version": o.Version,
			"pid":     os.Getpid(),
			"home":    o.Config.Home,
		})
	})
	server := tools.NewServer(tools.Deps{
		Store: st, Version: o.Version, Logger: o.logger(),
		Port: o.Config.Port, Assignee: o.Config.Assignee,
	})
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: o.logger()},
	))
	closeStreams := web.Register(mux, web.Deps{Store: st, Graph: web.StoreGraph(st), Logger: o.logger()})
	return hostCheck(o.Config, mux), closeStreams
}

// hostCheck rejects requests not addressed to 127.0.0.1:<port> or
// localhost:<port>, so a web page can't reach the daemon by DNS rebinding.
// A daemon listening beyond loopback is meant to be addressed by other
// names, so it checks nothing.
func hostCheck(cfg config.Config, next http.Handler) http.Handler {
	if !cfg.Loopback() {
		return next
	}
	p := strconv.Itoa(cfg.Port)
	allowed := map[string]bool{"127.0.0.1:" + p: true, "localhost:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "loom: Host must be 127.0.0.1:"+p+" or localhost:"+p, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
