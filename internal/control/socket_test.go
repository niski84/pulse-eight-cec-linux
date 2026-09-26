package control

import (
	"context"
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
