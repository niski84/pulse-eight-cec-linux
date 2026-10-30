// Package uinput provides a small, hardware-independent evdev output layer.
package uinput

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

const (
	eventTypeKey  = 0x01
	eventTypeSync = 0x00
	syncReport    = 0x00
)

const (
	keyEsc          = 1
	keyEnter        = 28
	keyUp           = 103
	keyDown         = 108
	keyLeft         = 105
	keyRight        = 106
	keyVolumeMute   = 113
	keyVolumeDown   = 114
	keyVolumeUp     = 115
	keyPower        = 116
	keyPause        = 201 // KEY_PAUSECD: media pause, not the keyboard Pause key
	keyStop         = 166 // KEY_STOPCD: media stop, not the browser/application Stop key
	keyMenu         = 139
	keyNextSong     = 163
	keyPlayPause    = 164
	keyPreviousSong = 165
	keyRecord       = 167
	keyRewind       = 168
	keyPlay         = 207
	keyFastForward  = 208
)

// KeyCodeToEvdev maps a CEC user-control code to a standard evdev key code.
func KeyCodeToEvdev(key cec.KeyCode) (uint16, bool) {
	switch key {
	case cec.KeySelect:
		return keyEnter, true
	case cec.KeyUp:
		return keyUp, true
	case cec.KeyDown:
		return keyDown, true
	case cec.KeyLeft:
		return keyLeft, true
	case cec.KeyRight:
		return keyRight, true
	case cec.KeyRootMenu, cec.KeyContentsMenu:
		return keyMenu, true
	case cec.KeySetupMenu:
		return keyMenu, true
	case cec.KeyExit:
		return keyEsc, true
	case cec.KeyPlay:
		return keyPlay, true
	case cec.KeyStop:
		return keyStop, true
	case cec.KeyPause:
		return keyPause, true
	case cec.KeyRecord:
		return keyRecord, true
	case cec.KeyRewind:
		return keyRewind, true
	case cec.KeyFastForward:
		return keyFastForward, true
	case cec.KeyPrevious:
		return keyPreviousSong, true
	case cec.KeyNext:
		return keyNextSong, true
	case cec.KeyPower:
		return keyPower, true
	case cec.KeyVolumeUp:
		return keyVolumeUp, true
	case cec.KeyVolumeDown:
		return keyVolumeDown, true
	case cec.KeyMute:
		return keyVolumeMute, true
	default:
		return 0, false
	}
}

// ActionToEvdev maps an application action using cec's default key map.
func ActionToEvdev(action cec.Action) (uint16, bool) {
	key, ok := cec.DefaultKeyMap().Code(action)
	if !ok {
		return 0, false
	}
	return KeyCodeToEvdev(key)
}

type output interface {
	io.Writer
	io.Closer
}

// Device is a virtual evdev keyboard backed by uinput.
type Device struct {
	mu      sync.Mutex
	output  output
	destroy func() error
	closed  bool
	keys    map[uint16]bool
}

func newDevice(output output, destroy func() error) *Device {
	return &Device{output: output, destroy: destroy}
}

// Close destroys the virtual input device and closes its underlying output.
// It is safe to call more than once.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true

	var closeErr error
	if d.destroy != nil {
		closeErr = d.destroy()
	}
	if err := d.output.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	if closeErr != nil {
		return fmt.Errorf("close uinput device: %w", closeErr)
	}
	return nil
}

// SendChord presses modifiers first and releases the complete chord in reverse
// order. The device lock keeps simultaneous remote/CLI actions from interleaving.
func (d *Device) SendChord(keys []uint16) (sendErr error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("uinput: device is closed")
	}
	if len(keys) == 0 || len(keys) > 8 {
		return errors.New("uinput: a chord requires 1 to 8 keys")
	}
	seen := make(map[uint16]bool)
	ordered := make([]uint16, 0, len(keys))
	for _, key := range keys {
		if key == 0 || key > 0x2ff || seen[key] {
			return fmt.Errorf("uinput: invalid or duplicate key %d", key)
		}
		if d.keys != nil && !d.keys[key] {
			return fmt.Errorf("uinput: key %d was not enabled on this keyboard", key)
		}
		seen[key] = true
		if isModifier(key) {
			ordered = append(ordered, key)
		}
	}
	for _, key := range keys {
		if !isModifier(key) {
			ordered = append(ordered, key)
		}
	}
	stamp := time.Now()
	write := func(kind, code uint16, value int32) error {
		return writeInputEvent(d.output, inputEvent{
			seconds: stamp.Unix(), microseconds: int64(stamp.Nanosecond() / 1000),
			eventType: kind, code: code, value: value,
		})
	}
	pressed := make([]uint16, 0, len(ordered))
	defer func() {
		// Try to release every attempted key, even when the output failed part
		// way through a chord. Return cleanup failures alongside the first error.
		for i := len(pressed) - 1; i >= 0; i-- {
			if err := write(eventTypeKey, pressed[i], 0); err != nil {
				sendErr = errors.Join(sendErr, fmt.Errorf("release key %d: %w", pressed[i], err))
			}
		}
		if err := write(eventTypeSync, syncReport, 0); err != nil {
			sendErr = errors.Join(sendErr, fmt.Errorf("sync key release: %w", err))
		}
	}()
	for _, key := range ordered {
		pressed = append(pressed, key)
		if err := write(eventTypeKey, key, 1); err != nil {
			return fmt.Errorf("press key %d: %w", key, err)
		}
	}
	if err := write(eventTypeSync, syncReport, 0); err != nil {
		return fmt.Errorf("sync key press: %w", err)
	}
	return nil
}

func isModifier(key uint16) bool {
	switch key {
	case 29, 97, 42, 54, 56, 100, 125, 126:
		return true
	}
	return false
}

// SendKey emits a press and release for a standard evdev key code.
func (d *Device) SendKey(key uint16) error {
	return d.SendChord([]uint16{key})
}

// SendKeyCode maps a CEC key and emits the corresponding Linux event.
func (d *Device) SendKeyCode(key cec.KeyCode) error {
	evdevKey, ok := KeyCodeToEvdev(key)
	if !ok {
		return fmt.Errorf("uinput: unknown CEC key 0x%02x", uint8(key))
	}
	return d.SendKey(evdevKey)
}

// SendAction maps and emits an application action.
func (d *Device) SendAction(action cec.Action) error {
	key, ok := ActionToEvdev(action)
	if !ok {
		return fmt.Errorf("uinput: unknown action %q", action)
	}
	return d.SendKey(key)
}
