package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/config"
)

func load(t *testing.T, toml string) (config.Config, error) {
	t.Helper()
	home := t.TempDir()
	if toml != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(toml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return config.Load(context.Background(), home)
}

func TestListenDefaultsToLoopback(t *testing.T) {
	c, err := load(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr() != "127.0.0.1" || !c.Loopback() {
		t.Fatalf("got %q, loopback %v", c.ListenAddr(), c.Loopback())
	}
}

func TestListenCanBeSet(t *testing.T) {
	for listen, loopback := range map[string]bool{"0.0.0.0": false, "192.168.1.5": false, "::": false, "127.0.0.2": true, "::1": true} {
		c, err := load(t, `listen = "`+listen+`"`)
		if err != nil {
			t.Fatalf("%s: %v", listen, err)
		}
		if c.ListenAddr() != listen || c.Loopback() != loopback {
			t.Errorf("%s: got %q, loopback %v", listen, c.ListenAddr(), c.Loopback())
		}
	}
}

func TestListenMustBeAnIP(t *testing.T) {
	_, err := load(t, `listen = "laptop.lan"`)
	if err == nil || !strings.Contains(err.Error(), "not an IP address") {
		t.Fatalf("got %v", err)
	}
}
