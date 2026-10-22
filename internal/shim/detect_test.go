package shim_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/HellRok/loom/internal/shim"
)

// fakeDaemon answers /healthz with body on a free 127.0.0.1 port.
func fakeDaemon(t *testing.T, body map[string]any) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).Port
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func loomHealth(version, home string) map[string]any {
	return map[string]any{"name": "loom", "version": version, "pid": 4242, "home": home}
}

func TestDetectWithNoDaemonSpawns(t *testing.T) {
	target := shim.Target{Home: "/h", Port: freePort(t), Version: "v1.0.0"}
	act, _, err := target.Detect(context.Background())
	if err != nil || act != shim.Spawn {
		t.Fatalf("got %v, %v; want Spawn", act, err)
	}
}

func TestDetectDecidesOnVersion(t *testing.T) {
	for _, tc := range []struct {
		name, self, daemon string
		want               shim.Action
	}{
		{"same release", "v1.2.0", "v1.2.0", shim.Use},
		{"older daemon", "v1.2.0", "v1.1.9", shim.Replace},
		{"newer daemon", "v1.2.0", "v1.10.0", shim.Use},
		{"older pseudo-version", "v1.2.1-0.20261022000000-abcdefabcdef", "v1.2.0", shim.Replace},
		{"same dev build", "devel+abc.2026-10-22T00:00:00Z", "devel+abc.2026-10-22T00:00:00Z", shim.Use},
		{"different dev build", "devel+abc.2026-10-22T00:00:00Z.dirty", "devel+abc.2026-10-22T00:00:00Z", shim.Replace},
		{"dev shim, release daemon", "devel+abc", "v1.2.0", shim.Replace},
		{"release shim, dev daemon", "v1.2.0", "devel+abc", shim.Replace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := fakeDaemon(t, loomHealth(tc.daemon, "/h"))
			act, h, err := shim.Target{Home: "/h", Port: port, Version: tc.self}.Detect(context.Background())
			if err != nil || act != tc.want {
				t.Fatalf("got %v, %v; want %v", act, err, tc.want)
			}
			if h.PID != 4242 || h.Version != tc.daemon {
				t.Fatalf("health not reported: %+v", h)
			}
		})
	}
}

func TestDetectRefusesAForeignLoomHome(t *testing.T) {
	port := fakeDaemon(t, loomHealth("v1.0.0", "/other/loom"))
	_, _, err := shim.Target{Home: "/mine/loom", Port: port, Version: "v1.0.0"}.Detect(context.Background())
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"/other/loom", "/mine/loom", "port", "/mine/loom/config.toml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDetectRejectsANonLoomResponder(t *testing.T) {
	for name, body := range map[string]map[string]any{
		"other json":  {"status": "ok"},
		"other named": {"name": "nginx"},
	} {
		t.Run(name, func(t *testing.T) {
			port := fakeDaemon(t, body)
			_, _, err := shim.Target{Home: "/h", Port: port, Version: "v1"}.Detect(context.Background())
			if err == nil {
				t.Fatal("want an error")
			}
			for _, want := range []string{strconv.Itoa(port), "`port`", "config.toml"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
	t.Run("not json", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)
		port := srv.Listener.Addr().(*net.TCPAddr).Port
		_, _, err := shim.Target{Home: "/h", Port: port, Version: "v1"}.Detect(context.Background())
		if err == nil || !strings.Contains(err.Error(), strconv.Itoa(port)) {
			t.Fatalf("got %v", err)
		}
	})
}
