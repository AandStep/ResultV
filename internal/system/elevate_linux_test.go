//go:build linux

package system

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseStatusRealUID(t *testing.T) {
	status := "Name:\tpkexec\nUmask:\t0022\nUid:\t1000\t0\t0\t0\nGid:\t1000\t1000\t1000\t1000\n"
	uid, ok := parseStatusRealUID(strings.NewReader(status))
	if !ok || uid != 1000 {
		t.Fatalf("got %d %v, want real uid 1000", uid, ok)
	}
	if _, ok := parseStatusRealUID(strings.NewReader("Name:\tx\n")); ok {
		t.Fatal("status without Uid line must not parse")
	}
}

func TestProcRealUIDOfSelf(t *testing.T) {
	uid, ok := procRealUID(os.Getpid())
	if !ok || uid != os.Getuid() {
		t.Fatalf("got %d %v, want %d", uid, ok, os.Getuid())
	}
}

func startExit(t *testing.T, code int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit "+strconv.Itoa(code))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func TestWaitElevationMapsPkexecExitCodes(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("real uid 0 is read as a successful elevation")
	}
	cases := map[int]error{126: ErrElevationCancelled, 127: ErrElevationDenied}
	for code, want := range cases {
		if err := waitElevation(startExit(t, code), time.Minute); !errors.Is(err, want) {
			t.Fatalf("exit %d: got %v, want %v", code, err, want)
		}
	}
	if err := waitElevation(startExit(t, 1), time.Minute); err == nil {
		t.Fatal("exit 1 must be an error")
	}
}

func TestWaitElevationTimeoutKillsPrompt(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("real uid 0 is read as a successful elevation")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := waitElevation(cmd, 300*time.Millisecond); !errors.Is(err, ErrElevationTimeout) {
		t.Fatalf("got %v, want timeout", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout did not fire promptly")
	}
}

func TestPkexecRelaunchArgs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/u/.config")
	args := pkexecRelaunchArgs("/usr/bin/resultv", []string{"--tray"}, 4242)
	joined := strings.Join(args, " ")
	for _, want := range []string{"XDG_CONFIG_HOME=/home/u/.config", "/usr/bin/resultv --elevated-from=4242 --tray"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q missing in %q", want, joined)
		}
	}
	if args[0] != "env" {
		t.Fatalf("args must start with env, got %q", args[0])
	}
}

func TestWaitForElevationParent(t *testing.T) {
	cmd := exec.Command("sleep", "0.5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	start := time.Now()
	WaitForElevationParent([]string{"resultv", ElevatedFromFlag + strconv.Itoa(cmd.Process.Pid)})
	if d := time.Since(start); d < 300*time.Millisecond || d > 5*time.Second {
		t.Fatalf("waited %v, want roughly the parent's lifetime", d)
	}

	start = time.Now()
	WaitForElevationParent([]string{"resultv"})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("no flag must not wait")
	}
}
