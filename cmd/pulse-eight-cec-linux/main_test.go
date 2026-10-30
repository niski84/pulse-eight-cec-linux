package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
	"github.com/niski84/pulse-eight-cec-linux/internal/control"
	"github.com/niski84/pulse-eight-cec-linux/internal/policy"
)

func TestForwardKeysSkipsUnknownAndPropagatesOutputFailure(t *testing.T) {
	events := make(chan cec.KeyCode, 3)
	events <- 0xff
	events <- cec.KeyUp
	events <- cec.KeyPause
	state := &bridgeState{}
	wantErr := errors.New("keyboard disconnected")
	var forwarded [][]uint16
	err := forwardKeys(context.Background(), events, nil, nil, func(keys []uint16) error {
		forwarded = append(forwarded, keys)
		if keys[0] == 201 {
			return wantErr
		}
		return nil
	}, state)
	if !errors.Is(err, wantErr) {
		t.Fatalf("forwardKeys error = %v", err)
	}
	if len(forwarded) != 2 || forwarded[0][0] != 103 || forwarded[1][0] != 201 {
		t.Fatalf("forwarded keys = %v", forwarded)
	}
	status := state.snapshot()
	if status.Received != 3 || status.Forwarded != 1 || status.Unsupported != 1 {
		t.Fatalf("incorrect counters: %+v", status)
	}
}

func TestForwardKeysReturnsTransportFailure(t *testing.T) {
	failures := make(chan error, 1)
	want := errors.New("adapter unplugged")
	failures <- want
	if err := forwardKeys(context.Background(), nil, failures, nil, nil, &bridgeState{}); !errors.Is(err, want) {
		t.Fatalf("forwardKeys error = %v", err)
	}
}

func TestActivityPolicyAppliesToRemoteAndLocalKeys(t *testing.T) {
	engine, err := policy.Load(strings.NewReader(`{"default_profile":"desktop","profiles":{"desktop":{},"browser":{"bindings":{"exit":{"keys":["KEY_LEFTALT","KEY_LEFT"]},"power":{"ignore":true}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan cec.KeyCode, 4)
	output := make(chan []uint16, 4)
	send := func(keys []uint16) error { output <- keys; return nil }
	state := &bridgeState{policy: engine}
	done := make(chan error, 1)
	go func() { done <- forwardKeys(ctx, events, nil, nil, send, state) }()
	check := func(want []uint16) {
		t.Helper()
		select {
		case got := <-output:
			if len(got) != len(want) {
				t.Fatalf("output = %v, want %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("output = %v, want %v", got, want)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("input was not forwarded")
		}
	}
	events <- cec.KeyExit
	check([]uint16{1})
	if err := engine.SetProfile("browser"); err != nil {
		t.Fatal(err)
	}
	events <- cec.KeyExit
	check([]uint16{56, 105})
	if err := dispatchCLI(engine, cec.ActionExit, send); err != nil {
		t.Fatal(err)
	}
	check([]uint16{56, 105})
	events <- cec.KeyPower
	deadline := time.Now().Add(time.Second)
	for state.snapshot().Ignored != 1 {
		if time.Now().After(deadline) {
			t.Fatal("explicitly ignored input was not counted")
		}
		time.Sleep(time.Millisecond)
	}
	trace := engine.Trace(0)
	if len(trace) != 4 || trace[0].Profile != "desktop" || trace[1].Profile != "browser" || trace[2].Source != "cli" || trace[3].Outcome != policy.Ignored {
		t.Fatalf("incorrect shared policy trace: %+v", trace)
	}
	select {
	case keys := <-output:
		t.Fatalf("ignored power key emitted %v", keys)
	default:
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Invoke the real CLI without opening any physical adapter or input device.
func TestCommandProcess(t *testing.T) {
	if os.Getenv("P8CEC_TEST_COMMAND") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"pulse-eight-cec"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func commandProcess(env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCommandProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	return cmd
}

func TestShadowDaemonStatusAndCLIHaveNoTransmitFallback(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "cec.sock")
	client := filepath.Join(dir, "cec-client")
	starts := filepath.Join(dir, "starts")
	config := filepath.Join(dir, "profiles.json")
	trigger := filepath.Join(dir, "next-key")
	if err := os.WriteFile(config, []byte(`{"default_profile":"desktop","profiles":{"desktop":{},"browser":{"bindings":{"exit":{"keys":["KEY_LEFTALT","KEY_LEFT"]}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/bash
echo start >> "$P8CEC_TEST_STARTS"
echo 'NOTICE: [ 1 ] CEC client registered: libCEC version = 6.0.2, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'waiting for input'
echo 'TRAFFIC: [ 2 ] << 01:44:04'
echo 'TRAFFIC: [ 3 ] >> 01:44:01'
echo 'TRAFFIC: [ 4 ] >> 01:44:ff'
while :; do
  if [ -f "$P8CEC_TEST_NEXT" ]; then
    rm "$P8CEC_TEST_NEXT"
    echo 'TRAFFIC: [ 5 ] >> 01:44:0d'
  fi
  if IFS= read -r -t 0.02 line; then [ "$line" = q ] && exit 0; fi
done
`
	if err := os.WriteFile(client, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	env := []string{"P8CEC_TEST_COMMAND=1", "P8CEC_BINARY=" + client,
		"P8CEC_SOCKET=" + socket, "P8CEC_UINPUT=/does/not/exist", "P8CEC_TEST_STARTS=" + starts,
		"P8CEC_CONFIG=" + config, "P8CEC_TEST_NEXT=" + trigger}
	cmd := commandProcess(env, "daemon", "--shadow")
	logFile, err := os.Create(filepath.Join(dir, "daemon.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon exit: %v", err)
			}
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("daemon did not stop on SIGTERM")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, err := control.ReadStatus(socket)
		if err == nil && status.Received == 2 {
			if !status.AdapterReady || !status.Shadow || status.InputReady || status.Forwarded != 0 || status.Unsupported != 1 {
				t.Fatalf("incorrect shadow status: %+v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, "daemon.log"))
			t.Fatalf("daemon did not report received input: status=%+v err=%v log=%s", status, err, log)
		}
		time.Sleep(10 * time.Millisecond)
	}
	output, err := commandProcess(env, "status").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `"daemon_running":true`) {
		t.Fatalf("status: %s, %v", output, err)
	}
	output, err = commandProcess(env, "key", "pause").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "shadow mode does not inject keys") {
		t.Fatalf("shadow key should fail without opening adapter: %s, %v", output, err)
	}
	output, err = commandProcess(env, "profiles").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `["browser","desktop"]`) {
		t.Fatalf("profiles: %s, %v", output, err)
	}
	output, err = commandProcess(env, "profile", "browser").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `"active_profile":"browser"`) {
		t.Fatalf("profile selection: %s, %v", output, err)
	}
	output, err = commandProcess(env, "profile", "missing").CombinedOutput()
	if err == nil {
		t.Fatalf("unknown profile accepted: %s", output)
	}
	if err := os.WriteFile(trigger, nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for {
		status, err := control.ReadStatus(socket)
		if err == nil && status.Received == 3 {
			if status.ActiveProfile != "browser" {
				t.Fatalf("wrong active profile after rejected change: %+v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("remote event after activity change not processed: %+v %v", status, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	output, err = commandProcess(env, "trace", "--limit", "1").CombinedOutput()
	if err != nil {
		t.Fatalf("trace: %s, %v", output, err)
	}
	var decisions []policy.Decision
	if err := json.Unmarshal(output, &decisions); err != nil || len(decisions) != 1 {
		t.Fatalf("invalid trace: %s, %v", output, err)
	}
	d := decisions[0]
	if d.Profile != "browser" || d.Input != "exit" || d.Outcome != policy.Shadow || !d.Explicit || d.Sequence != 3 || len(d.KeyNames) != 2 || d.KeyNames[0] != "KEY_LEFTALT" || d.KeyNames[1] != "KEY_LEFT" {
		t.Fatalf("remote did not use selected activity: %+v", d)
	}
	data, err := os.ReadFile(starts)
	if err != nil || strings.Count(string(data), "start") != 1 {
		t.Fatalf("CLI opened competing adapter: %s, %v", data, err)
	}
}

func TestExplainIsOfflineAndInvalidConfigFailsBeforeDevices(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "profiles.json")
	if err := os.WriteFile(config, []byte(`{"default_profile":"browser","profiles":{"browser":{"bindings":{"exit":{"keys":["KEY_LEFTALT","KEY_LEFT"]}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"P8CEC_TEST_COMMAND=1", "P8CEC_BINARY=/should/not/be/executed", "P8CEC_UINPUT=/does/not/exist", "P8CEC_CONFIG=" + config}
	output, err := commandProcess(env, "explain", "exit").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `"outcome":"preview"`) || !strings.Contains(string(output), `"KEY_LEFTALT"`) {
		t.Fatalf("offline explain: %s, %v", output, err)
	}
	output, err = commandProcess(env, "explain", "--profile", "missing", "exit").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unknown profile") {
		t.Fatalf("unknown profile preview: %s, %v", output, err)
	}
	if err := os.WriteFile(config, []byte(`{"default_profile":"browser","profiles":{"browser":{"bindings":{"exit":{"keys":["KEY_NOT_REAL"]}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	output, err = commandProcess(env, "daemon").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unknown key name") || strings.Contains(string(output), "open /does/not/exist") {
		t.Fatalf("invalid config reached hardware: %s, %v", output, err)
	}
}

func TestKeyWithoutDaemonDoesNotOpenAdapter(t *testing.T) {
	t.Setenv("P8CEC_SOCKET", filepath.Join(t.TempDir(), "absent.sock"))
	t.Setenv("P8CEC_BINARY", "/should/not/be/executed")
	err := runKey([]string{"up"})
	if err == nil || !strings.Contains(err.Error(), "no adapter fallback") {
		t.Fatalf("key error = %v", err)
	}
}
