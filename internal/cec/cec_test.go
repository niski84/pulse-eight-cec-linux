package cec

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeAdapter struct {
	openCalls  int
	closeCalls int
	keys       []KeyCode
	openErr    error
	closeErr   error
	sendErr    error
}

func (f *fakeAdapter) Open(context.Context) error {
	f.openCalls++
	return f.openErr
}

func (f *fakeAdapter) Close() error {
	f.closeCalls++
	return f.closeErr
}

func (f *fakeAdapter) SendKey(_ context.Context, key KeyCode) error {
	f.keys = append(f.keys, key)
	return f.sendErr
}

func TestDefaultKeyMap(t *testing.T) {
	keys := DefaultKeyMap()

	tests := map[Action]KeyCode{
		ActionUp:         KeyUp,
		ActionPause:      KeyPause,
		ActionVolumeDown: KeyVolumeDown,
		ActionMute:       KeyMute,
	}
	for action, want := range tests {
		got, ok := keys.Code(action)
		if !ok {
			t.Fatalf("Code(%q) reported missing action", action)
		}
		if got != want {
			t.Errorf("Code(%q) = %#x, want %#x", action, got, want)
		}
	}

	if _, ok := keys.Code(Action("not-an-action")); ok {
		t.Fatal("unknown action unexpectedly resolved")
	}
}

func TestSessionLifecycleAndKeyMapping(t *testing.T) {
	fake := &fakeAdapter{}
	session, err := NewSession(fake, DefaultKeyMap())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	ctx := context.Background()
	if err := session.Open(ctx); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := session.Open(ctx); err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := session.SendAction(ctx, ActionPause); err != nil {
		t.Fatalf("SendAction() error = %v", err)
	}
	if !reflect.DeepEqual(fake.keys, []KeyCode{KeyPause}) {
		t.Fatalf("sent keys = %#v, want %#v", fake.keys, []KeyCode{KeyPause})
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if fake.openCalls != 1 || fake.closeCalls != 1 {
		t.Fatalf("lifecycle calls = open:%d close:%d, want open:1 close:1", fake.openCalls, fake.closeCalls)
	}
}

func TestSessionRejectsInvalidUse(t *testing.T) {
	if _, err := NewSession(nil, DefaultKeyMap()); err == nil {
		t.Fatal("NewSession(nil, ...) returned nil error")
	}
	if _, err := NewSession(&fakeAdapter{}, nil); err == nil {
		t.Fatal("NewSession(..., nil) returned nil error")
	}

	fake := &fakeAdapter{}
	session, err := NewSession(fake, DefaultKeyMap())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := session.SendAction(context.Background(), ActionPause); err == nil {
		t.Fatal("SendAction() succeeded before Open()")
	}
	if err := session.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := session.SendAction(context.Background(), Action("unknown")); err == nil {
		t.Fatal("SendAction() succeeded for unknown action")
	}
}

func TestSessionPropagatesAdapterErrors(t *testing.T) {
	openErr := errors.New("open failed")
	fake := &fakeAdapter{openErr: openErr}
	session, err := NewSession(fake, DefaultKeyMap())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := session.Open(context.Background()); !errors.Is(err, openErr) {
		t.Fatalf("Open() error = %v, want wrapped %v", err, openErr)
	}

	fake = &fakeAdapter{}
	session, err = NewSession(fake, DefaultKeyMap())
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := session.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	sendErr := errors.New("send failed")
	fake.sendErr = sendErr
	if err := session.SendAction(context.Background(), ActionPlay); !errors.Is(err, sendErr) {
		t.Fatalf("SendAction() error = %v, want wrapped %v", err, sendErr)
	}

	closeErr := errors.New("close failed")
	fake.closeErr = closeErr
	if err := session.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want wrapped %v", err, closeErr)
	}
}
