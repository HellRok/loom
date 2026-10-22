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
	"github.com/HellRok/loom/internal/store"
)

const usage = `usage: loom <command> [flags]

commands:
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
	home, err := config.Home()
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	cfg, err := config.Load(ctx, home)
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
