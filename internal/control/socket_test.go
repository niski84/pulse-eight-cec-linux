package control

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

func TestSocketRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sock")
	received := make(chan cec.Action, 1)
	server, err := New(path, func(_ context.Context, action cec.Action) error {
		received <- action
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for {
		if err := Send(path, cec.ActionPause); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control socket did not become ready")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case action := <-received:
		if action != cec.ActionPause {
			t.Fatalf("received action %q, want %q", action, cec.ActionPause)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for action")
	}
}

func TestLiveSocketCannotBeReplacedAndStatusDoesNotInject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	injected := make(chan cec.Action, 1)
	server, err := New(path, func(_ context.Context, action cec.Action) error {
		injected <- action
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	server.WithStatus(func() Status { return Status{AdapterReady: true, InputReady: true} })
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for {
		status, err := ReadStatus(path)
		if err == nil {
			if !status.AdapterReady || !status.InputReady {
				t.Fatalf("incorrect status: %+v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("status socket never became ready")
		}
		time.Sleep(time.Millisecond)
	}
	other, err := New(path, func(context.Context, cec.Action) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Run(ctx); err == nil {
		t.Fatal("second server replaced a live endpoint")
	}
	if err := Send(path, cec.ActionUp); err != nil {
		t.Fatal(err)
	}
	if got := <-injected; got != cec.ActionUp {
		t.Fatalf("wrong injected action: %s", got)
	}
	select {
	case action := <-injected:
		t.Fatalf("status request injected extra action: %s", action)
	default:
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions: %v, %v", info, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestStaleSocketCanBeRecovered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := New(path, func(context.Context, cec.Action) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for {
		if err := Send(path, cec.ActionUp); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale socket was not recovered")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
