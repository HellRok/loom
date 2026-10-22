// Package config locates the Loom home and reads its optional config.toml.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the effective configuration: config.toml over the defaults.
type Config struct {
	Home     string `toml:"-"`
	Port     int    `toml:"port"`
	Assignee string `toml:"assignee"`
}

// DefaultPort is the port the daemon binds when config.toml sets none.
const DefaultPort = 7337

// Home returns the Loom home: $LOOM_HOME, else ~/.loom.
func Home() (string, error) {
	if h := os.Getenv("LOOM_HOME"); h != "" {
		return filepath.Abs(h)
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating Loom home: %w (set LOOM_HOME)", err)
	}
	return filepath.Join(u, ".loom"), nil
}

// Load reads home/config.toml if it exists and fills in defaults.
func Load(ctx context.Context, home string) (Config, error) {
	c := Config{Home: home}
	path := filepath.Join(home, "config.toml")
	md, err := toml.DecodeFile(path, &c)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return Config{}, fmt.Errorf("%s: unknown keys %v (known: port, assignee)", path, keys)
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.Port < 1 || c.Port > 65535 {
		return Config{}, fmt.Errorf("%s: port %d is out of range", path, c.Port)
	}
	if c.Assignee == "" {
		c.Assignee = defaultAssignee(ctx)
	}
	return c, nil
}

// defaultAssignee is git's global user.name, else $USER.
func defaultAssignee(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "user.name").Output()
	if name := strings.TrimSpace(string(out)); err == nil && name != "" {
		return name
	}
	return os.Getenv("USER")
}
