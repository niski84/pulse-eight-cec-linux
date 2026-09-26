package cecclient

import (
	"testing"

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
