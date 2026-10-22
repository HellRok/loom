package shim_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/HellRok/loom/internal/shim"
)

// fakeDaemonProcess is a live process standing in for a daemon: home's
// lockfile names lockPID and /healthz on the returned port reports
// healthPID. Use proc.Process.Pid for the real one. exited closes when the
// process ends.
func fakeDaemonProcess(t *testing.T, lockPID, healthPID func(real int) int) (target shim.Target, real int, exited <-chan struct{}) {
	t.Helper()
	proc := exec.Command("sleep", "60")
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { proc.Wait(); close(done) }()
	t.Cleanup(func() { proc.Process.Kill(); <-done })

	pid := proc.Process.Pid
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "loom.lock"), []byte(strconv.Itoa(lockPID(pid))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := loomHealth("v1.0.0", home)
	h["pid"] = healthPID(pid)
	return shim.Target{Home: home, Port: fakeDaemon(t, h), Version: "v1.0.0"}, pid, done
}

func same(pid int) int  { return pid }
func other(pid int) int { return pid + 100000 }

func TestStopSignalsTheDaemonAndWaitsForItToExit(t *testing.T) {
	target, _, exited := fakeDaemonProcess(t, same, same)

	stopped, err := target.Stop(context.Background(), 0)
	if !stopped || err != nil {
		t.Fatalf("got %v, %v", stopped, err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("Stop returned before the daemon exited")
	}
}

func TestStopSignalsNothingUnlessLockAndHealthzAgree(t *testing.T) {
	for name, pids := range map[string][2]func(int) int{
		"healthz reports another pid": {same, other},
		"lockfile names another pid":  {other, same},
	} {
		t.Run(name, func(t *testing.T) {
			target, _, exited := fakeDaemonProcess(t, pids[0], pids[1])
			stopped, err := target.Stop(context.Background(), 0)
			if stopped || err != nil {
				t.Fatalf("got %v, %v; want not running", stopped, err)
			}
			assertAlive(t, exited)
		})
	}
	t.Run("no lockfile", func(t *testing.T) {
		stopped, err := shim.Target{Home: t.TempDir(), Port: freePort(t)}.Stop(context.Background(), 0)
		if stopped || err != nil {
			t.Fatalf("got %v, %v; want not running", stopped, err)
		}
	})
}

// A shim replacing an older daemon stops only the one it saw, so a racing
// shim never stops the daemon its rival just started.
func TestStopWithAnExpectedPidLeavesAnyOtherDaemonAlone(t *testing.T) {
	target, real, exited := fakeDaemonProcess(t, same, same)
	stopped, err := target.Stop(context.Background(), other(real))
	if stopped || err != nil {
		t.Fatalf("got %v, %v; want not running", stopped, err)
	}
	assertAlive(t, exited)
}

func assertAlive(t *testing.T, exited <-chan struct{}) {
	t.Helper()
	select {
	case <-exited:
		t.Fatal("signalled a process it should have left alone")
	case <-time.After(100 * time.Millisecond):
	}
}
