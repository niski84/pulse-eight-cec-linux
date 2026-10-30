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
		{name: "media pause", cec: cec.KeyPause, want: 201},
		{name: "media stop", cec: cec.KeyStop, want: 166},
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

func TestChordEmitsModifiersFirstAndReleasesInReverse(t *testing.T) {
	fake := &fakeOutput{}
	device := newDevice(fake, nil)
	// Accept modifier order independently of how the chord was written.
	if err := device.SendChord([]uint16{106, 29, 42}); err != nil {
		t.Fatal(err)
	}
	var got [][3]int
	for offset := 0; offset < len(fake.data); offset += 24 {
		got = append(got, [3]int{
			int(binary.LittleEndian.Uint16(fake.data[offset+16 : offset+18])),
			int(binary.LittleEndian.Uint16(fake.data[offset+18 : offset+20])),
			int(int32(binary.LittleEndian.Uint32(fake.data[offset+20 : offset+24]))),
		})
	}
	want := [][3]int{{1, 29, 1}, {1, 42, 1}, {1, 106, 1}, {0, 0, 0}, {1, 106, 0}, {1, 42, 0}, {1, 29, 0}, {0, 0, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chord events = %v, want %v", got, want)
	}
	device.keys = map[uint16]bool{29: true}
	before := len(fake.data)
	if err := device.SendChord([]uint16{29, 106}); err == nil || len(fake.data) != before {
		t.Fatal("unsupported chord wrote input before validation")
	}
}

type failsOnceOutput struct {
	fakeOutput
	calls int
}

func (f *failsOnceOutput) Write(data []byte) (int, error) {
	f.calls++
	if f.calls == 2 {
		return 0, errors.New("output failed")
	}
	return f.fakeOutput.Write(data)
}

func TestChordFailureAttemptsToReleaseModifiers(t *testing.T) {
	fake := &failsOnceOutput{}
	device := newDevice(fake, nil)
	if err := device.SendChord([]uint16{29, 106}); err == nil {
		t.Fatal("output error was hidden")
	}
	// The failed arrow press is followed by attempted releases of the arrow
	// and Ctrl, plus SYN_REPORT, so a transient write failure doesn't latch Ctrl.
	if len(fake.data) != 4*24 {
		t.Fatalf("cleanup events missing: %d bytes", len(fake.data))
	}
	ctrlRelease := fake.data[2*24 : 3*24]
	if binary.LittleEndian.Uint16(ctrlRelease[18:20]) != 29 || binary.LittleEndian.Uint32(ctrlRelease[20:24]) != 0 {
		t.Fatal("Ctrl was not released after output failure")
	}
}
