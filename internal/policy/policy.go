// Package policy resolves CEC inputs against explicit activity profiles.
// It is hardware independent: callers supply the output function to Dispatch.
package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
	"github.com/niski84/pulse-eight-cec-linux/internal/uinput"
)

const maxConfigSize = 1 << 20
const traceCapacity = 50

type rawConfig struct {
	DefaultProfile string                `json:"default_profile"`
	Profiles       map[string]rawProfile `json:"profiles"`
}
type rawProfile struct {
	Bindings map[string]rawBinding `json:"bindings"`
}
type rawBinding struct {
	Keys   []string `json:"keys,omitempty"`
	Ignore bool     `json:"ignore,omitempty"`
}
type binding struct {
	keys   []uint16
	ignore bool
}

// Outcome describes the policy result for one input.
type Outcome string

const (
	Forwarded Outcome = "forwarded"
	Preview   Outcome = "preview"
	Shadow    Outcome = "shadow"
	Ignored   Outcome = "ignored"
	Unmapped  Outcome = "unmapped"
	Failed    Outcome = "error"
)

// Decision is a pure resolution or a recorded dispatch result.
type Decision struct {
	Sequence  uint64   `json:"sequence,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
	Input     string   `json:"input"`
	Profile   string   `json:"profile"`
	Source    string   `json:"source,omitempty"`
	Outcome   Outcome  `json:"outcome"`
	Explicit  bool     `json:"explicit"`
	Keys      []uint16 `json:"key_codes,omitempty"`
	KeyNames  []string `json:"keys,omitempty"`
	Reason    string   `json:"reason"`
}

// Engine owns a validated, immutable copy of the profile configuration.
type Engine struct {
	mu       sync.Mutex
	active   string
	profiles map[string]map[uint8]binding
	trace    []Decision
	sequence uint64
}

var actions = func() map[string]uint8 {
	result := make(map[string]uint8)
	for action, code := range cec.DefaultKeyMap() {
		result[string(action)] = uint8(code)
	}
	return result
}()

var actionNames = func() []string {
	result := make([]string, 0, len(actions))
	for name := range actions {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}()

func readJSONValue(dec *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting exceeds 64 levels")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = true
			if err := readJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := readJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
}

func checkJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := readJSONValue(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func inputID(s string) (uint8, error) {
	if n, ok := actions[s]; ok {
		return n, nil
	}
	if len(s) == 4 && strings.HasPrefix(s, "0x") {
		n, e := strconv.ParseUint(s[2:], 16, 8)
		if e == nil {
			return uint8(n), nil
		}
	}
	return 0, fmt.Errorf("unknown input %q (use an action name or 0xNN)", s)
}

// ParseInput resolves an action name or explicit hexadecimal operand (0xNN).
func ParseInput(s string) (cec.KeyCode, error) {
	id, err := inputID(s)
	return cec.KeyCode(id), err
}
func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

var evdevNames = func() map[string]uint16 {
	m := map[string]uint16{"KEY_ESC": 1, "KEY_ENTER": 28, "KEY_SPACE": 57, "KEY_BACKSPACE": 14, "KEY_TAB": 15, "KEY_HOME": 102, "KEY_END": 107, "KEY_PAGEUP": 104, "KEY_PAGEDOWN": 109, "KEY_DELETE": 111, "KEY_LEFT": 105, "KEY_RIGHT": 106, "KEY_UP": 103, "KEY_DOWN": 108, "KEY_VOLUMEUP": 115, "KEY_VOLUMEDOWN": 114, "KEY_MUTE": 113, "KEY_POWER": 116, "KEY_MENU": 139, "KEY_PLAY": 207, "KEY_PLAYPAUSE": 164, "KEY_STOP": 128, "KEY_STOPCD": 166, "KEY_PAUSE": 119, "KEY_PAUSECD": 201, "KEY_RECORD": 167, "KEY_REWIND": 168, "KEY_FASTFORWARD": 208, "KEY_NEXTSONG": 163, "KEY_PREVIOUSSONG": 165}
	for i, code := range []uint16{30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50, 49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44} {
		m[fmt.Sprintf("KEY_%c", 'A'+i)] = code
		if i < 10 {
			m[fmt.Sprintf("KEY_%d", i)] = uint16([]int{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}[i])
		}
	}
	for i := 1; i <= 12; i++ {
		code := uint16(59 + i - 1)
		if i == 11 {
			code = 87
		}
		if i == 12 {
			code = 88
		}
		m[fmt.Sprintf("KEY_F%d", i)] = code
	}
	for n, c := range map[string]uint16{"LEFTCTRL": 29, "RIGHTCTRL": 97, "LEFTSHIFT": 42, "RIGHTSHIFT": 54, "LEFTALT": 56, "RIGHTALT": 100, "LEFTMETA": 125, "RIGHTMETA": 126} {
		m["KEY_"+n] = c
	}
	return m
}()

func parseKey(s string) (uint16, error) {
	if strings.TrimSpace(s) != s || s == "" {
		return 0, fmt.Errorf("invalid key name %q", s)
	}
	n, ok := evdevNames[s]
	if !ok {
		return 0, fmt.Errorf("unknown key name %q", s)
	}
	return n, nil
}

// Load decodes and validates JSON from a bounded reader.
func Load(r io.Reader) (*Engine, error) {
	data, e := io.ReadAll(io.LimitReader(r, maxConfigSize+1))
	if e != nil {
		return nil, e
	}
	if len(data) > maxConfigSize {
		return nil, errors.New("policy: config exceeds 1 MiB")
	}
	if err := checkJSON(data); err != nil {
		return nil, fmt.Errorf("policy: invalid JSON structure: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw rawConfig
	if e = dec.Decode(&raw); e != nil {
		return nil, fmt.Errorf("policy: decode config: %w", e)
	}
	var extra any
	if e = dec.Decode(&extra); e != io.EOF {
		if e == nil {
			return nil, errors.New("policy: trailing JSON value")
		}
		return nil, fmt.Errorf("policy: trailing data: %w", e)
	}
	if len(raw.Profiles) == 0 {
		return nil, errors.New("policy: profiles must not be empty")
	}
	if !validName(raw.DefaultProfile) {
		return nil, fmt.Errorf("policy: invalid default profile %q", raw.DefaultProfile)
	}
	if _, ok := raw.Profiles[raw.DefaultProfile]; !ok {
		return nil, fmt.Errorf("policy: unknown default profile %q", raw.DefaultProfile)
	}
	eg := &Engine{active: raw.DefaultProfile, profiles: make(map[string]map[uint8]binding)}
	for name, p := range raw.Profiles {
		if !validName(name) {
			return nil, fmt.Errorf("policy: invalid profile name %q", name)
		}
		compiled := make(map[uint8]binding)
		for input, b := range p.Bindings {
			id, err := inputID(input)
			if err != nil {
				return nil, err
			}
			if _, ok := compiled[id]; ok {
				return nil, fmt.Errorf("policy: duplicate aliases for input 0x%02x in profile %q", id, name)
			}
			if b.Ignore {
				if len(b.Keys) != 0 {
					return nil, fmt.Errorf("policy: binding %q cannot combine ignore and keys", input)
				}
				compiled[id] = binding{ignore: true}
				continue
			}
			if len(b.Keys) == 0 {
				return nil, fmt.Errorf("policy: binding %q requires keys or ignore:true", input)
			}
			if len(b.Keys) > 8 {
				return nil, fmt.Errorf("policy: binding %q has more than 8 keys", input)
			}
			keys := make([]uint16, 0, len(b.Keys))
			seen := map[uint16]bool{}
			for _, k := range b.Keys {
				code, err := parseKey(k)
				if err != nil {
					return nil, err
				}
				if seen[code] {
					return nil, fmt.Errorf("policy: duplicate key %q in binding %q", k, input)
				}
				seen[code] = true
				keys = append(keys, code)
			}
			compiled[id] = binding{keys: keys}
		}
		eg.profiles[name] = compiled
	}
	return eg, nil
}

// LoadFile loads a config file.
func LoadFile(path string) (*Engine, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return Load(f)
}

// NewDefault creates the built-in desktop profile.
func NewDefault() *Engine {
	return &Engine{active: "desktop", profiles: map[string]map[uint8]binding{"desktop": {}}}
}
func (e *Engine) Profiles() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.profiles))
	for n := range e.profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
func (e *Engine) ActiveProfile() string { e.mu.Lock(); defer e.mu.Unlock(); return e.active }
func (e *Engine) SetProfile(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.profiles[name]; !ok {
		return fmt.Errorf("policy: unknown profile %q", name)
	}
	e.active = name
	return nil
}
func (e *Engine) resolve(id uint8, profile string) Decision {
	d := Decision{Input: fmt.Sprintf("0x%02x", id), Profile: profile}
	for _, a := range actionNames {
		if actions[a] == id {
			d.Input = a
			break
		}
	}
	if b, ok := e.profiles[profile][id]; ok {
		d.Explicit = true
		if b.ignore {
			d.Outcome = Ignored
			d.Reason = "suppressed by explicit profile binding"
			return d
		}
		d.Keys = append([]uint16(nil), b.keys...)
		d.KeyNames = names(d.Keys)
		d.Outcome = Forwarded
		d.Reason = "explicit profile chord"
		return d
	}
	if code, ok := uinput.KeyCodeToEvdev(cec.KeyCode(id)); ok {
		d.Keys = []uint16{code}
		d.KeyNames = names(d.Keys)
		d.Outcome = Forwarded
		d.Reason = "default CEC-to-evdev mapping"
		return d
	}
	d.Outcome = Unmapped
	d.Reason = "no profile binding or default mapping"
	return d
}

var evdevCanonicalNames = func() map[uint16]string {
	names := make([]string, 0, len(evdevNames))
	for name := range evdevNames {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make(map[uint16]string)
	for _, name := range names {
		code := evdevNames[name]
		if _, exists := result[code]; !exists {
			result[code] = name
		}
	}
	return result
}()

func names(codes []uint16) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = evdevCanonicalNames[c]
		if out[i] == "" {
			out[i] = fmt.Sprintf("KEY_%d", c)
		}
	}
	return out
}

// Explain resolves without changing profile selection, sequence, or trace.
func (e *Engine) Explain(key uint8, profile string) (Decision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if profile == "" {
		profile = e.active
	}
	if _, ok := e.profiles[profile]; !ok {
		return Decision{}, fmt.Errorf("policy: unknown profile %q", profile)
	}
	d := e.resolve(key, profile)
	if d.Outcome == Forwarded {
		d.Outcome = Preview
		d.Reason = "preview: " + d.Reason
	}
	return d, nil
}
func clone(d Decision) Decision {
	d.Keys = append([]uint16(nil), d.Keys...)
	d.KeyNames = append([]string(nil), d.KeyNames...)
	return d
}
func (e *Engine) record(d Decision) Decision {
	e.sequence++
	d.Sequence = e.sequence
	d.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	e.trace = append(e.trace, clone(d))
	if len(e.trace) > traceCapacity {
		e.trace = append([]Decision(nil), e.trace[len(e.trace)-traceCapacity:]...)
	}
	return clone(d)
}

// Dispatch resolves, optionally emits, and records one input atomically with profile selection.
func (e *Engine) Dispatch(key uint8, source string, shadow bool, output func([]uint16) error) (Decision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d := e.resolve(key, e.active)
	d.Source = source
	var err error
	switch d.Outcome {
	case Ignored, Unmapped:
	case Forwarded:
		if shadow {
			d.Outcome = Shadow
			d.Reason += "; shadow mode would emit " + strings.Join(d.KeyNames, "+")
		} else if output == nil {
			err = errors.New("policy: output callback is nil")
			d.Outcome = Failed
			d.Reason = err.Error()
		} else if err = output(append([]uint16(nil), d.Keys...)); err != nil {
			d.Outcome = Failed
			d.Reason = "output failed: " + err.Error()
		} else {
			d.Reason += "; emitted " + strings.Join(d.KeyNames, "+")
		}
	}
	return e.record(d), err
}

// Trace returns the newest limit records in sequence order (limit <= 0 means all).
func (e *Engine) Trace(limit int) []Decision {
	e.mu.Lock()
	defer e.mu.Unlock()
	start := 0
	if limit > 0 && limit < len(e.trace) {
		start = len(e.trace) - limit
	}
	out := make([]Decision, 0, len(e.trace)-start)
	for _, d := range e.trace[start:] {
		out = append(out, clone(d))
	}
	return out
}

// RequiredKeys returns sorted unique evdev codes needed by defaults and configured chords.
func (e *Engine) RequiredKeys() []uint16 {
	set := map[uint16]bool{}
	for _, id := range cec.DefaultKeyMap() {
		if c, ok := uinput.KeyCodeToEvdev(id); ok {
			set[c] = true
		}
	}
	for _, p := range e.profiles {
		for _, b := range p {
			for _, k := range b.keys {
				set[k] = true
			}
		}
	}
	out := make([]uint16, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
