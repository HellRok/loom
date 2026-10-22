// Command loom is a local-first issue tracker that agents drive over MCP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/HellRok/loom/internal/buildinfo"
	"github.com/HellRok/loom/internal/config"
	"github.com/HellRok/loom/internal/daemon"
	"github.com/HellRok/loom/internal/shim"
	"github.com/HellRok/loom/internal/store"
)

const usage = `usage: loom <command> [flags]

commands:
  mcp     serve MCP over stdio, starting the daemon if needed
  stop    stop the daemon
  project move or delete a Project (mv, rm)
  serve   run the daemon (normally started for you by the shim)

Run 'loom <command> -h' for a command's flags.
`

func main() {
	fs := flag.NewFlagSet("loom", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), usage) }
	fs.Parse(os.Args[1:])
	if fs.NArg() == 0 {
		fs.Usage()
		os.Exit(2)
	}
	cmd, args := fs.Arg(0), fs.Args()[1:]
	switch cmd {
	case "mcp":
		os.Exit(mcpShim(args))
	case "stop":
		os.Exit(stopDaemon(args))
	case "project":
		os.Exit(projectCmd(args))
	case "serve":
		os.Exit(serve(args))
	default:
		fmt.Fprintf(fs.Output(), "loom: unknown command %q\n\n", cmd)
		fs.Usage()
		os.Exit(2)
	}
}

func serve(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom serve\n\nRun the daemon. Configure it with $LOOM_HOME and config.toml.\n")
	}
	fs.Parse(args)
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	cfg, err := loadConfig(ctx)
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	version := buildinfo.Version()
	logger.Info("loom serve", "version", version, "pid", os.Getpid(), "home", cfg.Home, "port", cfg.Port, "assignee", cfg.Assignee)

	err = daemon.Run(ctx, daemon.Options{Config: cfg, Version: version, Logger: logger})

	tooNew, isTooNew := errors.AsType[*store.SchemaTooNewError](err)
	switch {
	case err == nil:
		logger.Info("stopped")
		return 0
	case errors.Is(err, daemon.ErrAlreadyRunning), errors.Is(err, daemon.ErrPortInUse):
		logger.Info("not starting", "reason", err)
		return 0
	case isTooNew:
		logger.Error("refusing to open database", "db_version", tooNew.DB, "binary_version", tooNew.Binary, "err", err)
		return 1
	default:
		logger.Error("serve failed", "err", err)
		return 1
	}
}

// mcpShim runs `loom mcp`. Its stdout carries MCP only; anything else goes
// to stderr.
func mcpShim(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom mcp\n\nServe MCP over stdio for an agent harness, starting the daemon if it isn't running.\n")
	}
	fs.Parse(args)
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	t, err := target(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "loom mcp:", err)
		return 1
	}
	s, err := shim.New(t)
	if err != nil {
		fmt.Fprintln(os.Stderr, "loom mcp:", err)
		return 1
	}
	if err := s.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "loom mcp:", err)
		return 1
	}
	return 0
}

// stopDaemon runs `loom stop`.
func stopDaemon(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), "usage: loom stop\n\nStop the daemon gracefully, letting in-flight calls finish.\n")
	}
	fs.Parse(args)
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	ctx := context.Background()
	t, err := target(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "loom stop:", err)
		return 1
	}
	stopped, err := t.Stop(ctx, 0)
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "loom stop:", err)
		return 1
	case stopped:
		fmt.Println("stopped")
	default:
		fmt.Println("not running")
	}
	return 0
}

// loadConfig reads the Loom home's effective config.
func loadConfig(ctx context.Context) (config.Config, error) {
	home, err := config.Home()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(ctx, home)
}

// target is the daemon this build of loom expects for the Loom home.
func target(ctx context.Context) (shim.Target, error) {
	cfg, err := loadConfig(ctx)
	if err != nil {
		return shim.Target{}, err
	}
	return shim.Target{Home: cfg.Home, Port: cfg.Port, Version: buildinfo.Version()}, nil
}
