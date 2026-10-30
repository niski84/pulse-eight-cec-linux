package policy

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
	"github.com/niski84/pulse-eight-cec-linux/internal/uinput"
)

func loadText(t *testing.T, s string) *Engine {
	t.Helper()
	e, err := Load(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestProfilesDefaultsAndUnmapped(t *testing.T) {
	e := loadText(t, `{"default_profile":"desktop","profiles":{"desktop":{"bindings":{}},"browser":{"bindings":{"exit":{"keys":["KEY_LEFTALT","KEY_LEFT"]},"power":{"ignore":true}}}}}`)
	if err := e.SetProfile("browser"); err != nil {
		t.Fatal(err)
	}
	d, err := e.Explain(uint8(cec.KeyExit), "")
	if err != nil || d.Outcome != Preview || !d.Explicit || len(d.Keys) != 2 {
		t.Fatalf("profile chord = %+v", d)
	}
	d, err = e.Explain(uint8(cec.KeyUp), "")
	want, _ := uinput.KeyCodeToEvdev(cec.KeyUp)
	if d.Explicit || len(d.Keys) != 1 || d.Keys[0] != want {
		t.Fatalf("default fallback = %+v", d)
	}
	d, err = e.Explain(0x72, "")
	if d.Outcome != Unmapped {
		t.Fatalf("raw input = %+v", d)
	}
	d, err = e.Explain(uint8(cec.KeyPower), "")
	if d.Outcome != Ignored {
		t.Fatalf("ignore = %+v", d)
	}
	if err := e.SetProfile("missing"); err == nil || e.ActiveProfile() != "browser" {
		t.Fatal("invalid profile changed selection")
	}
	if got := e.Profiles(); len(got) != 2 || got[0] != "browser" {
		t.Fatalf("profiles not sorted: %v", got)
	}
}

func TestDispatchShadowErrorAndTraceCopies(t *testing.T) {
	e := NewDefault()
	called := 0
	d, err := e.Dispatch(uint8(cec.KeyUp), "remote", true, func([]uint16) error { called++; return nil })
	if err != nil || d.Outcome != Shadow || called != 0 {
		t.Fatalf("shadow dispatch: %+v err=%v called=%d", d, err, called)
	}
	wantErr := errors.New("device failed")
	d, err = e.Dispatch(uint8(cec.KeyUp), "cli", false, func([]uint16) error { return wantErr })
	if !errors.Is(err, wantErr) || d.Outcome != Failed {
		t.Fatalf("failed dispatch: %+v err=%v", d, err)
	}
	if got := e.Trace(2); len(got) != 2 || got[0].Sequence != 1 || got[1].Outcome != Failed {
		t.Fatalf("trace order: %+v", got)
	}
	got := e.Trace(1)
	got[0].Keys[0] = 999
	got[0].KeyNames[0] = "corrupt"
	if e.Trace(1)[0].Keys[0] == 999 {
		t.Fatal("trace keys alias internal state")
	}
	if _, err := e.Dispatch(uint8(cec.KeyUp), "cli", false, nil); err == nil {
		t.Fatal("nil output succeeded")
	}
}

func TestExplainDoesNotRecordAndTraceBound(t *testing.T) {
	e := NewDefault()
	_, _ = e.Explain(0x72, "")
	if len(e.Trace(0)) != 0 {
		t.Fatal("Explain mutated trace")
	}
	for i := 0; i < 55; i++ {
		_, _ = e.Dispatch(0x72, "test", false, nil)
	}
	got := e.Trace(0)
	if len(got) != traceCapacity || got[0].Sequence != 6 || got[len(got)-1].Sequence != 55 {
		t.Fatalf("bounded trace: len=%d first=%d last=%d", len(got), got[0].Sequence, got[len(got)-1].Sequence)
	}
}

func TestConcurrentProfileSelectionAndDispatch(t *testing.T) {
	e := loadText(t, `{"default_profile":"a","profiles":{"a":{"bindings":{"0x72":{"keys":["KEY_A"]}}},"b":{"bindings":{"0x72":{"keys":["KEY_B"]}}}}}`)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if i%2 == 0 {
					_ = e.SetProfile("a")
				} else {
					_ = e.SetProfile("b")
				}
				_, err := e.Dispatch(0x72, "test", false, func(c []uint16) error {
					if len(c) != 1 || (c[0] != 30 && c[0] != 48) {
						t.Errorf("invalid output %v", c)
					}
					return nil
				})
				if err != nil {
					t.Error(err)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestStrictConfigValidation(t *testing.T) {
	bad := []string{
		`{"default_profile":"absent","profiles":{"x":{"bindings":{}}}}`,
		`{"default_profile":"x","profiles":{"bad name":{"bindings":{}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"keys":["KEY_A"],"oops":true}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"keys":["KEY_NOT_REAL"]}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"ignore":true,"keys":["KEY_A"]}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"keys":["KEY_A","KEY_A"]}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"keys":["KEY_A"]},"0x0d":{"ignore":true}}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{}}}} {}`,
		`{"default_profile":"x","default_profile":"y","profiles":{"x":{"bindings":{}}}}`,
		`{"default_profile":"x","profiles":{"x":{"bindings":{"exit":{"ignore":true},"exit":{"keys":["KEY_A"]}}}}}`,
	}
	for _, s := range bad {
		if _, err := Load(strings.NewReader(s)); err == nil {
			t.Errorf("accepted invalid config: %s", s)
		}
	}
	long := strings.Repeat(" ", maxConfigSize+1)
	if _, err := Load(strings.NewReader(long)); err == nil {
		t.Fatal("accepted oversized config")
	}
}

func TestKeyCodeBoundariesAndDefaultMap(t *testing.T) {
	for name, want := range map[string]uint16{"KEY_F1": 59, "KEY_F10": 68, "KEY_F11": 87, "KEY_F12": 88, "KEY_PAUSE": 119, "KEY_PAUSECD": 201, "KEY_STOP": 128, "KEY_STOPCD": 166, "KEY_PLAYPAUSE": 164, "KEY_B": 48, "KEY_0": 11, "KEY_9": 10} {
		got, err := parseKey(name)
		if err != nil || got != want {
			t.Errorf("%s = %d, %v; want %d", name, got, err, want)
		}
	}
	e := NewDefault()
	for _, tc := range []struct {
		input uint8
		want  uint16
	}{{0x00, 28}, {0x01, 103}, {0x02, 108}, {0x03, 105}, {0x04, 106}, {0x09, 139}, {0x0a, 139}, {0x0b, 139}, {0x0d, 1}, {0x44, 207}, {0x45, 166}, {0x46, 201}, {0x47, 167}, {0x48, 168}, {0x49, 208}, {0x4b, 163}, {0x4c, 165}, {0x40, 116}, {0x41, 115}, {0x42, 114}, {0x43, 113}} {
		d, err := e.Explain(tc.input, "desktop")
		if err != nil || d.Outcome != Preview || len(d.Keys) != 1 || d.Keys[0] != tc.want {
			t.Errorf("default input 0x%02x = %+v, %v; want code %d", tc.input, d, err, tc.want)
		}
	}
}

func TestExplainProfileIgnoredDispatchAndRequiredKeys(t *testing.T) {
	e := loadText(t, `{"default_profile":"desktop","profiles":{"desktop":{"bindings":{"0x72":{"keys":["KEY_LEFTCTRL","KEY_X"]},"power":{"ignore":true}}}}}`)
	if _, err := e.Explain(0x72, "unknown"); err == nil {
		t.Fatal("Explain accepted unknown profile")
	}
	called := false
	d, err := e.Dispatch(uint8(cec.KeyPower), "remote", false, func([]uint16) error { called = true; return nil })
	if err != nil || called || d.Outcome != Ignored {
		t.Fatalf("ignored dispatch = %+v err=%v called=%v", d, err, called)
	}
	keys := e.RequiredKeys()
	seen := map[uint16]bool{}
	for _, k := range keys {
		seen[k] = true
	}
	if !seen[29] || !seen[45] {
		t.Fatalf("required keys omit configured chord: %v", keys)
	}
}

func TestDispatchReturnedRecordMatchesTrace(t *testing.T) {
	e := NewDefault()
	d, err := e.Dispatch(uint8(cec.KeyUp), "cli", false, func([]uint16) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	last := e.Trace(1)[0]
	if d.Sequence != last.Sequence || d.Timestamp == "" || d.Timestamp != last.Timestamp || d.Reason != last.Reason || d.Outcome != last.Outcome {
		t.Fatalf("dispatch decision differs from trace: %+v vs %+v", d, last)
	}
}
