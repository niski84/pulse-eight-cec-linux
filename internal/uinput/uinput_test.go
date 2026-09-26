package uinput

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

type fakeOutput struct {
	data      []byte
	closeErr  error
	closeCall int
}

func (f *fakeOutput) Write(data []byte) (int, error) {
	f.data = append(f.data, data...)
	return len(data), nil
}

func (f *fakeOutput) Close() error {
	f.closeCall++
	return f.closeErr
}

func TestKeyMapping(t *testing.T) {
	tests := []struct {
		name string
		cec  cec.KeyCode
		want uint16
	}{
		{name: "select", cec: cec.KeySelect, want: keyEnter},
		{name: "pause", cec: cec.KeyPause, want: keyPause},
		{name: "volume up", cec: cec.KeyVolumeUp, want: keyVolumeUp},
		{name: "mute", cec: cec.KeyMute, want: keyVolumeMute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := KeyCodeToEvdev(test.cec)
			if !ok || got != test.want {
				t.Fatalf("KeyCodeToEvdev(%#x) = %#x, %t; want %#x, true", test.cec, got, ok, test.want)
			}
		})
	}

	if _, ok := KeyCodeToEvdev(cec.KeyCode(0xFF)); ok {
		t.Fatal("unknown CEC key unexpectedly mapped")
	}
	if got, ok := ActionToEvdev(cec.ActionPause); !ok || got != keyPause {
		t.Fatalf("ActionToEvdev(pause) = %#x, %t; want %#x, true", got, ok, keyPause)
	}
}

func TestDeviceSendsPressReleaseAndClosesOnce(t *testing.T) {
	fake := &fakeOutput{}
	device := newDevice(fake, nil)

	if err := device.SendAction(cec.ActionPause); err != nil {
		t.Fatalf("SendAction() error = %v", err)
	}
	if err := device.SendKeyCode(cec.KeyMute); err != nil {
		t.Fatalf("SendKeyCode() error = %v", err)
	}
	if len(fake.data) != 2*4*24 {
		t.Fatalf("wrote %d bytes, want %d", len(fake.data), 2*4*24)
	}

	var values []int32
	var types []uint16
	for offset := 0; offset < len(fake.data); offset += 24 {
		types = append(types, binary.LittleEndian.Uint16(fake.data[offset+16:offset+18]))
		values = append(values, int32(binary.LittleEndian.Uint32(fake.data[offset+20:offset+24])))
	}
	if want := []uint16{eventTypeKey, eventTypeSync, eventTypeKey, eventTypeSync, eventTypeKey, eventTypeSync, eventTypeKey, eventTypeSync}; !reflect.DeepEqual(types, want) {
		t.Fatalf("event types = %#v, want %#v", types, want)
	}
	if want := []int32{1, 0, 0, 0, 1, 0, 0, 0}; !reflect.DeepEqual(values, want) {
		t.Fatalf("event values = %#v, want %#v", values, want)
	}

	if err := device.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := device.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if fake.closeCall != 1 {
		t.Fatalf("close calls = %d, want 1", fake.closeCall)
	}
	if err := device.SendKey(keyPause); err == nil {
		t.Fatal("SendKey() succeeded after Close()")
	}
}

func TestDeviceCloseErrorAndUnknownAction(t *testing.T) {
	closeErr := errors.New("close failed")
	fake := &fakeOutput{closeErr: closeErr}
	device := newDevice(fake, nil)
	if err := device.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want wrapped %v", err, closeErr)
	}

	device = newDevice(&fakeOutput{}, nil)
	if err := device.SendAction(cec.Action("unknown")); err == nil {
		t.Fatal("SendAction() succeeded for unknown action")
	}
}
