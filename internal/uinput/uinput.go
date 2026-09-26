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
	keyPause        = 119
	keyStop         = 128
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

func (d *Device) sendKey(key uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("uinput: device is closed")
	}

	stamp := time.Now()
	for _, event := range []inputEvent{
		{seconds: stamp.Unix(), microseconds: int64(stamp.Nanosecond() / 1000), eventType: eventTypeKey, code: key, value: 1},
		{seconds: stamp.Unix(), microseconds: int64(stamp.Nanosecond() / 1000), eventType: eventTypeSync, code: syncReport},
		{seconds: stamp.Unix(), microseconds: int64(stamp.Nanosecond() / 1000), eventType: eventTypeKey, code: key, value: 0},
		{seconds: stamp.Unix(), microseconds: int64(stamp.Nanosecond() / 1000), eventType: eventTypeSync, code: syncReport},
	} {
		if err := writeInputEvent(d.output, event); err != nil {
			return fmt.Errorf("send evdev key %d: %w", key, err)
		}
	}
	return nil
}

// SendKey emits a press and release for a standard evdev key code.
func (d *Device) SendKey(key uint16) error {
	return d.sendKey(key)
}

// SendKeyCode maps a CEC key and emits the corresponding Linux event.
func (d *Device) SendKeyCode(key cec.KeyCode) error {
	evdevKey, ok := KeyCodeToEvdev(key)
	if !ok {
		return fmt.Errorf("uinput: unknown CEC key 0x%02x", uint8(key))
	}
	return d.sendKey(evdevKey)
}

// SendAction maps and emits an application action.
func (d *Device) SendAction(action cec.Action) error {
	key, ok := ActionToEvdev(action)
	if !ok {
		return fmt.Errorf("uinput: unknown action %q", action)
	}
	return d.sendKey(key)
}
