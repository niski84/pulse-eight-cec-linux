// Package cecclient adapts the libCEC cec-client utility to the hardware
// independent cec.Adapter contract.
package cecclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

var trafficPattern = regexp.MustCompile(`(?i)<<\s+([0-9a-f]{2}):44:([0-9a-f]{2})`)

type Config struct {
	Binary string
	Device string
	Source uint8
	Target uint8
	Debug  int
}

type Adapter struct {
	cfg Config

	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	closed     chan struct{}
	events     chan cec.KeyCode
	eventsOnce sync.Once
	opened     bool
}

func New(cfg Config) *Adapter {
	if cfg.Binary == "" {
		cfg.Binary = "cec-client"
	}
	if cfg.Debug == 0 {
		cfg.Debug = 8
	}
	return &Adapter{cfg: cfg, events: make(chan cec.KeyCode, 32)}
}

func (a *Adapter) Open(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.opened {
		return nil
	}
	args := []string{"-d", strconv.Itoa(a.cfg.Debug)}
	if a.cfg.Device != "" {
		args = append(args, a.cfg.Device)
	}
	cmd := exec.Command(a.cfg.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("cec-client stdout: %w", err)
	}
	cmd.Stderr = cmd.Stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("cec-client stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cec-client: %w", err)
	}
	a.cmd = cmd
	a.stdin = stdin
	a.closed = make(chan struct{})
	a.events = make(chan cec.KeyCode, 32)
	a.eventsOnce = sync.Once{}
	a.opened = true
	go a.readOutput(stdout)

	select {
	case <-ctx.Done():
		_ = a.closeLocked()
		return ctx.Err()
	case <-time.After(250 * time.Millisecond):
		return nil
	}
}

func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeLocked()
}

func (a *Adapter) closeLocked() error {
	if !a.opened {
		return nil
	}
	close(a.closed)
	_, _ = io.WriteString(a.stdin, "q\n")
	_ = a.stdin.Close()
	err := a.cmd.Wait()
	a.cmd = nil
	a.stdin = nil
	a.opened = false
	if err != nil {
		return fmt.Errorf("stop cec-client: %w", err)
	}
	return nil
}

func (a *Adapter) SendKey(ctx context.Context, key cec.KeyCode) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.opened || a.stdin == nil {
		return errors.New("cec-client is not open")
	}
	frame := Frame(a.cfg.Source, a.cfg.Target, key)
	result := make(chan error, 1)
	go func() {
		_, err := io.WriteString(a.stdin, "tx "+frame+"\n")
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			return fmt.Errorf("write cec frame: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Adapter) Events() <-chan cec.KeyCode {
	return a.events
}

func Frame(source, target uint8, key cec.KeyCode) string {
	header := (source&0x0f)<<4 | (target & 0x0f)
	return fmt.Sprintf("%02x:44:%02x", header, uint8(key))
}

func (a *Adapter) readOutput(reader io.Reader) {
	defer a.eventsOnce.Do(func() { close(a.events) })
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		match := trafficPattern.FindStringSubmatch(scanner.Text())
		if len(match) != 3 {
			continue
		}
		value, err := strconv.ParseUint(match[2], 16, 8)
		if err != nil {
			continue
		}
		select {
		case a.events <- cec.KeyCode(value):
		case <-a.closed:
			return
		default:
		}
	}
}

type Factory struct {
	Config Config
}

func (f Factory) NewAdapter(context.Context) (cec.Adapter, error) {
	if strings.TrimSpace(f.Config.Binary) == "" {
		f.Config.Binary = "cec-client"
	}
	return New(f.Config), nil
}
