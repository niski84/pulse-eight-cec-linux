package main

import (
	"context"
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
)

func TestForwardKeysSkipsUnknownAndPropagatesOutputFailure(t *testing.T) {
	events := make(chan cec.KeyCode, 3)
	events <- 0xff
	events <- cec.KeyUp
	events <- cec.KeyPause
	state := &bridgeState{}
	wantErr := errors.New("keyboard disconnected")
	var forwarded []cec.KeyCode
	err := forwardKeys(context.Background(), events, nil, nil, func(key cec.KeyCode) error {
		forwarded = append(forwarded, key)
		if key == cec.KeyPause {
			return wantErr
		}
		return nil
	}, state)
	if !errors.Is(err, wantErr) {
		t.Fatalf("forwardKeys error = %v", err)
	}
	if len(forwarded) != 2 || forwarded[0] != cec.KeyUp || forwarded[1] != cec.KeyPause {
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
	script := `#!/bin/sh
echo start >> "$P8CEC_TEST_STARTS"
echo 'NOTICE: [ 1 ] CEC client registered: libCEC version = 6.0.2, logical address(es) = Recorder 1 (1) , base device: TV (0)'
echo 'waiting for input'
echo 'TRAFFIC: [ 2 ] << 01:44:04'
echo 'TRAFFIC: [ 3 ] >> 01:44:01'
echo 'TRAFFIC: [ 4 ] >> 01:44:ff'
while IFS= read -r line; do [ "$line" = q ] && exit 0; done
`
	if err := os.WriteFile(client, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	env := []string{"P8CEC_TEST_COMMAND=1", "P8CEC_BINARY=" + client,
		"P8CEC_SOCKET=" + socket, "P8CEC_UINPUT=/does/not/exist", "P8CEC_TEST_STARTS=" + starts}
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
	data, err := os.ReadFile(starts)
	if err != nil || strings.Count(string(data), "start") != 1 {
		t.Fatalf("CLI opened competing adapter: %s, %v", data, err)
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
