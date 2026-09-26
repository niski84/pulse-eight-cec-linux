// Package cec defines the hardware-independent contract used to control a CEC
// adapter.
package cec

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// KeyCode is a CEC user-control code.
type KeyCode uint8

const (
	KeySelect       KeyCode = 0x00
	KeyUp           KeyCode = 0x01
	KeyDown         KeyCode = 0x02
	KeyLeft         KeyCode = 0x03
	KeyRight        KeyCode = 0x04
	KeyRootMenu     KeyCode = 0x09
	KeySetupMenu    KeyCode = 0x0A
	KeyContentsMenu KeyCode = 0x0B
	KeyExit         KeyCode = 0x0D
	KeyPlay         KeyCode = 0x44
	KeyStop         KeyCode = 0x45
	KeyPause        KeyCode = 0x46
	KeyRecord       KeyCode = 0x47
	KeyRewind       KeyCode = 0x48
	KeyFastForward  KeyCode = 0x49
	KeyPrevious     KeyCode = 0x4C
	KeyNext         KeyCode = 0x4B
	KeyPower        KeyCode = 0x40
	KeyVolumeUp     KeyCode = 0x41
	KeyVolumeDown   KeyCode = 0x42
	KeyMute         KeyCode = 0x43
)

// Action is a stable application-level name for a CEC key.
type Action string

const (
	ActionSelect       Action = "select"
	ActionUp           Action = "up"
	ActionDown         Action = "down"
	ActionLeft         Action = "left"
	ActionRight        Action = "right"
	ActionRootMenu     Action = "root_menu"
	ActionSetupMenu    Action = "setup_menu"
	ActionContentsMenu Action = "contents_menu"
	ActionExit         Action = "exit"
	ActionPlay         Action = "play"
	ActionStop         Action = "stop"
	ActionPause        Action = "pause"
	ActionRecord       Action = "record"
	ActionRewind       Action = "rewind"
	ActionFastForward  Action = "fast_forward"
	ActionPrevious     Action = "previous"
	ActionNext         Action = "next"
	ActionPower        Action = "power"
	ActionVolumeUp     Action = "volume_up"
	ActionVolumeDown   Action = "volume_down"
	ActionMute         Action = "mute"
)

// KeyMap translates application actions into CEC key codes.
type KeyMap map[Action]KeyCode

// DefaultKeyMap returns the standard action mapping used by the package.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		ActionSelect:       KeySelect,
		ActionUp:           KeyUp,
		ActionDown:         KeyDown,
		ActionLeft:         KeyLeft,
		ActionRight:        KeyRight,
		ActionRootMenu:     KeyRootMenu,
		ActionSetupMenu:    KeySetupMenu,
		ActionContentsMenu: KeyContentsMenu,
		ActionExit:         KeyExit,
		ActionPlay:         KeyPlay,
		ActionStop:         KeyStop,
		ActionPause:        KeyPause,
		ActionRecord:       KeyRecord,
		ActionRewind:       KeyRewind,
		ActionFastForward:  KeyFastForward,
		ActionPrevious:     KeyPrevious,
		ActionNext:         KeyNext,
		ActionPower:        KeyPower,
		ActionVolumeUp:     KeyVolumeUp,
		ActionVolumeDown:   KeyVolumeDown,
		ActionMute:         KeyMute,
	}
}

// Code resolves an action to a CEC key code.
func (m KeyMap) Code(action Action) (KeyCode, bool) {
	code, ok := m[action]
	return code, ok
}

// Adapter is the hardware boundary for a CEC transmitter.
type Adapter interface {
	Open(context.Context) error
	Close() error
	SendKey(context.Context, KeyCode) error
}

// AdapterFactory creates adapters without exposing a hardware implementation.
type AdapterFactory interface {
	NewAdapter(context.Context) (Adapter, error)
}

// Session owns an adapter's open/close lifecycle and sends mapped keys.
type Session struct {
	adapter Adapter
	keys    KeyMap

	mu     sync.Mutex
	opened bool
}

// NewSession creates a session around an adapter and key map.
func NewSession(adapter Adapter, keys KeyMap) (*Session, error) {
	if adapter == nil {
		return nil, errors.New("cec: adapter is nil")
	}
	if keys == nil {
		return nil, errors.New("cec: key map is nil")
	}
	return &Session{adapter: adapter, keys: keys}, nil
}

// Open starts the adapter. An already-open session is left unchanged.
func (s *Session) Open(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opened {
		return nil
	}
	if err := s.adapter.Open(ctx); err != nil {
		return fmt.Errorf("open adapter: %w", err)
	}
	s.opened = true
	return nil
}

// Close stops the adapter. An already-closed session is left unchanged.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened {
		return nil
	}
	if err := s.adapter.Close(); err != nil {
		return fmt.Errorf("close adapter: %w", err)
	}
	s.opened = false
	return nil
}

// SendAction resolves and sends an application action.
func (s *Session) SendAction(ctx context.Context, action Action) error {
	code, ok := s.keys.Code(action)
	if !ok {
		return fmt.Errorf("cec: unknown action %q", action)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.opened {
		return errors.New("cec: adapter is not open")
	}
	if err := s.adapter.SendKey(ctx, code); err != nil {
		return fmt.Errorf("send %q: %w", action, err)
	}
	return nil
}
