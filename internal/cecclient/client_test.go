package cecclient

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

func TestFrame(t *testing.T) {
	if got := Frame(0, 1, cec.KeyPause); got != "01:44:46" {
		t.Fatalf("Frame() = %q, want %q", got, "01:44:46")
	}
	if got := Frame(1, 0, cec.KeyPlay); got != "10:44:44" {
		t.Fatalf("Frame() = %q, want %q", got, "10:44:44")
	}
}

func TestOpenParsesOnlyDirectedInboundKeyTraffic(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeClient(t, `printf '%s\n' "$*" > "$ARGS_FILE"
echo 'NOTICE: CEC client registered: libCEC, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'NOTICE: waiting for input'
echo 'TRAFFIC: [ 1234 ] << 01:44:46'
echo 'TRAFFIC: [ 1234 ] >> 01:44:46'
echo 'TRAFFIC: [ 1234 ] >> 04:44:44'
echo 'TRAFFIC: [ 1234 ] >> 01:44:GG'
echo 'TRAFFIC: [ 1234 ] >> 01:44:46 trailing'
while IFS= read -r line; do
  [ "$line" = q ] && exit 0
done`, map[string]string{"ARGS_FILE": argsFile})
	a := New(Config{Binary: bin, Target: 1, ReadyTimeout: time.Second})
	if err := a.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := a.Events()
	select {
	case got := <-events:
		if got != cec.KeyPause {
			t.Fatalf("event = %x, want %x", got, cec.KeyPause)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbound event")
	}
	select {
	case got := <-events:
		t.Fatalf("unexpected event %x", got)
	case <-time.After(30 * time.Millisecond):
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-t r") {
		t.Fatalf("arguments %q do not select recorder type", args)
	}
	if !strings.Contains(string(args), "-d 10") {
		t.Fatalf("arguments %q do not enable traffic and notice output", args)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenReadinessTimeoutAndCancellation(t *testing.T) {
	bin := fakeClient(t, `exec sleep 5`, nil)
	a := New(Config{Binary: bin, ReadyTimeout: 40 * time.Millisecond, ShutdownTimeout: 40 * time.Millisecond})
	start := time.Now()
	err := a.Open(context.Background())
	if err == nil || !strings.Contains(err.Error(), "readiness timed out") {
		t.Fatalf("Open error = %v, want readiness timeout", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timed out Open took %s", elapsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Open(ctx); err == nil {
		t.Fatal("Open with cancelled context succeeded")
	}
}

func TestOpenPromptWithoutRegisteredAddressFails(t *testing.T) {
	bin := fakeClient(t, `echo 'NOTICE: waiting for input'
while IFS= read -r line; do [ "$line" = q ] && exit 0; done`, nil)
	a := New(Config{Binary: bin, ReadyTimeout: 40 * time.Millisecond, ShutdownTimeout: 100 * time.Millisecond})
	err := a.Open(context.Background())
	if err == nil || !strings.Contains(err.Error(), "readiness timed out") {
		t.Fatalf("Open error = %v, want failure without registered address", err)
	}
}

func TestUnexpectedExitReportedAndReopenWorks(t *testing.T) {
	bin := fakeClient(t, `echo 'NOTICE: CEC client registered: libCEC, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'NOTICE: waiting for input'
count=0
[ -f "$COUNT_FILE" ] && count=$(cat "$COUNT_FILE")
count=$((count + 1))
echo "$count" > "$COUNT_FILE"
if [ "$count" -eq 1 ]; then sleep 0.1; exit 7; fi
while IFS= read -r line; do [ "$line" = q ] && exit 0; done`, map[string]string{"COUNT_FILE": filepath.Join(t.TempDir(), "count")})
	a := New(Config{Binary: bin, ReadyTimeout: time.Second})
	if err := a.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err, ok := <-a.Errors():
		if !ok || err == nil || !strings.Contains(err.Error(), "exited") {
			t.Fatalf("Errors() = %v, %v; want subprocess exit", err, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("unexpected exit was not reported")
	}
	select {
	case _, ok := <-a.Events():
		if ok {
			t.Fatal("Events channel remained open after process exit")
		}
	case <-time.After(time.Second):
		t.Fatal("Events channel was not closed after process exit")
	}
	if err := a.Open(context.Background()); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseKillsUnresponsiveProcessWithinBound(t *testing.T) {
	bin := fakeClient(t, `echo 'NOTICE: CEC client registered: libCEC, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'NOTICE: waiting for input'
exec sleep 5`, nil)
	a := New(Config{Binary: bin, ReadyTimeout: time.Second, ShutdownTimeout: 40 * time.Millisecond})
	if err := a.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close took %s despite shutdown timeout", elapsed)
	}
}

func TestCloseKillsProcessGroupWithInheritedOutputPipe(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	bin := fakeClient(t, `sleep 30 &
echo $! > "$CHILD_PID_FILE"
echo 'NOTICE: CEC client registered: libCEC, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'NOTICE: waiting for input'
trap '' TERM
while IFS= read -r line; do :; done`, map[string]string{"CHILD_PID_FILE": childPIDFile})
	t.Cleanup(func() {
		pidBytes, err := os.ReadFile(childPIDFile)
		if err == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidBytes))); parseErr == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	a := New(Config{Binary: bin, ReadyTimeout: time.Second, ShutdownTimeout: 40 * time.Millisecond})
	if err := a.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Close took %s with a child holding stdout open", elapsed)
	}
}

func fakeClient(t *testing.T, body string, env map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cec-client")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	// Make test-specific paths available to the child without shell interpolation.
	if len(env) != 0 {
		wrapper := filepath.Join(t.TempDir(), "wrapper")
		prefix := "#!/bin/sh\n"
		for key, value := range env {
			prefix += key + "='" + strings.ReplaceAll(value, "'", "'\\''") + "'\nexport " + key + "\n"
		}
		prefix += "exec '" + strings.ReplaceAll(path, "'", "'\\''") + "' \"$@\"\n"
		if err := os.WriteFile(wrapper, []byte(prefix), 0700); err != nil {
			t.Fatal(err)
		}
		return wrapper
	}
	return path
}
