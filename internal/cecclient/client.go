// Package cecclient adapts the libCEC cec-client utility to the hardware
// independent cec.Adapter contract.
package cecclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
)

// libCEC prints received frames with >> and transmitted frames with <<.
// Match only complete inbound key-press frames so debug output and our own
// transmissions can never be interpreted as user input.
var trafficPattern = regexp.MustCompile(`(?i)TRAFFIC:\s*\[[^\]]*\]\s*>>\s*[0-9a-f]([0-9a-f]):44:([0-9a-f]{2})\s*$`)
var registeredAddressesPattern = regexp.MustCompile(`(?i)logical address(?:\(es\))?\s*=\s*(.*?)(?:,\s*base device|$)`)
var logicalAddressPattern = regexp.MustCompile(`\(([0-9a-f])\)`)

const (
	defaultReadyTimeout    = 30 * time.Second
	defaultShutdownTimeout = 2 * time.Second
)

type Config struct {
	Binary          string
	Device          string
	Source          uint8
	Target          uint8
	Debug           int
	ReadyTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output io.ReadCloser
	events chan cec.KeyCode
	errors chan error
	ready  chan struct{}
	done   chan struct{}

	readyOnce sync.Once
	stopping  atomic.Bool
	addresses map[uint8]struct{}
	mu        sync.Mutex
	err       error
}

type Adapter struct {
	cfg Config
	mu  sync.Mutex
	// current belongs to one process generation. A reader only owns and closes
	// that generation's channels, so a late exit cannot affect a later Open.
	current *process
	opening bool
}

func New(cfg Config) *Adapter {
	if cfg.Binary == "" {
		cfg.Binary = "cec-client"
	}
	if cfg.Debug == 0 {
		cfg.Debug = 8
	}
	// Traffic lines carry frames; NOTICE lines report the address cec-client
	// actually registered when its preferred logical address was unavailable.
	cfg.Debug |= 2 | 8
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	return &Adapter{cfg: cfg}
}

func (a *Adapter) Open(ctx context.Context) error {
	a.mu.Lock()
	if a.opening {
		a.mu.Unlock()
		return errors.New("cec-client is already opening")
	}
	if a.current != nil {
		select {
		case <-a.current.done:
			a.current = nil
		default:
			a.mu.Unlock()
			return nil
		}
	}
	a.opening = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.opening = false
		a.mu.Unlock()
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	s := &process{
		events:    make(chan cec.KeyCode, 32),
		errors:    make(chan error, 1),
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
		addresses: make(map[uint8]struct{}),
	}

	args := []string{"-d", strconv.Itoa(a.cfg.Debug), "-t", "r"}
	if a.cfg.Device != "" {
		args = append(args, a.cfg.Device)
	}
	cmd := exec.Command(a.cfg.Binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	outR, outW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cec-client output pipe: %w", err)
	}
	cmd.Stdout, cmd.Stderr = outW, outW
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return fmt.Errorf("cec-client stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = outR.Close()
		_ = outW.Close()
		return fmt.Errorf("start cec-client: %w", err)
	}
	_ = outW.Close() // the child owns the remaining pipe writers
	s.cmd, s.stdin, s.output = cmd, stdin, outR
	a.mu.Lock()
	a.current = s
	a.mu.Unlock()
	go s.readOutput(outR)

	timer := time.NewTimer(a.cfg.ReadyTimeout)
	defer timer.Stop()
	select {
	case <-s.ready:
		select {
		case <-s.done:
			return fmt.Errorf("cec-client exited before readiness completed: %w", s.failure())
		default:
			return nil
		}
	case <-s.done:
		return fmt.Errorf("cec-client exited before readiness: %w", s.failure())
	case <-ctx.Done():
		_ = a.stop(s)
		return ctx.Err()
	case <-timer.C:
		_ = a.stop(s)
		return fmt.Errorf("cec-client readiness timed out after %s", a.cfg.ReadyTimeout)
	}
}

func (a *Adapter) Close() error {
	a.mu.Lock()
	s := a.current
	a.mu.Unlock()
	if s == nil {
		return nil
	}
	return a.stop(s)
}

func (a *Adapter) stop(s *process) error {
	alreadyStopping := s.stopping.Swap(true)
	if s.stdin != nil {
		// A process that stopped reading commands must not make Close block
		// before the shutdown timer can trigger the kill fallback.
		go func() {
			_, _ = io.WriteString(s.stdin, "q\n")
			_ = s.stdin.Close()
		}()
	}
	timer := time.NewTimer(a.cfg.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		if s.cmd != nil && s.cmd.Process != nil {
			// Kill the process group so grandchildren cannot keep the combined
			// stdout/stderr pipe open after cec-client itself exits.
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			_ = s.cmd.Process.Kill()
		}
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.output != nil {
			_ = s.output.Close()
		}
		<-s.done
	}
	a.clear(s)
	s.mu.Lock()
	priorErr := s.err
	s.mu.Unlock()
	if priorErr != nil && !alreadyStopping {
		return fmt.Errorf("stop cec-client: %w", priorErr)
	}
	return nil
}

func (a *Adapter) clear(s *process) {
	a.mu.Lock()
	if a.current == s {
		a.current = nil
	}
	a.mu.Unlock()
}

func (s *process) readOutput(reader io.ReadCloser) {
	scanner := bufio.NewScanner(reader)
	// Debug lines can include long adapter diagnostics.
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	promptSeen := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "ERROR:") {
			log.Printf("cec-client: %s", line)
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "client registered") {
			if match := registeredAddressesPattern.FindStringSubmatch(line); len(match) == 2 {
				for _, address := range logicalAddressPattern.FindAllStringSubmatch(match[1], -1) {
					value, err := strconv.ParseUint(address[1], 16, 4)
					if err == nil {
						s.addresses[uint8(value)] = struct{}{}
					}
				}
			}
		} else if strings.Contains(lower, "waiting for input") {
			promptSeen = true
		}
		if promptSeen && len(s.addresses) > 0 {
			s.readyOnce.Do(func() { close(s.ready) })
		}
		match := trafficPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		destination, err := strconv.ParseUint(match[1], 16, 4)
		if err != nil {
			continue
		}
		if _, local := s.addresses[uint8(destination)]; !local {
			continue
		}
		value, err := strconv.ParseUint(match[2], 16, 8)
		if err != nil {
			continue
		}
		select {
		case s.events <- cec.KeyCode(value):
		default:
			// Retain traffic flow while reporting overflow through Errors.
			s.report(errors.New("cec-client key event buffer full; dropping received key"))
		}
	}
	_ = reader.Close()
	var readErr error
	if err := scanner.Err(); err != nil {
		readErr = fmt.Errorf("read cec-client output: %w", err)
		if !s.stopping.Load() {
			s.report(readErr)
			// A scan error must not wait indefinitely for a child whose output
			// we can no longer drain.
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	waitErr := s.cmd.Wait()
	processErr := waitErr
	if processErr == nil && readErr != nil {
		processErr = readErr
	}
	if !s.stopping.Load() {
		if processErr == nil {
			processErr = errors.New("cec-client exited unexpectedly")
		}
		s.report(fmt.Errorf("cec-client process exited: %w", processErr))
	} else {
		processErr = nil
	}
	s.mu.Lock()
	s.err = processErr
	s.mu.Unlock()
	close(s.events)
	close(s.errors)
	close(s.done)
}

func (s *process) report(err error) {
	select {
	case s.errors <- err:
	default:
		log.Printf("cec-client reported additional transport error: %v", err)
	}
}

func (s *process) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	return errors.New("process stopped")
}

func (a *Adapter) SendKey(ctx context.Context, key cec.KeyCode) error {
	a.mu.Lock()
	s := a.current
	a.mu.Unlock()
	if s == nil {
		return errors.New("cec-client is not open")
	}
	select {
	case <-s.ready:
	case <-s.done:
		return fmt.Errorf("cec-client is not running: %w", s.failure())
	case <-ctx.Done():
		return ctx.Err()
	}
	result := make(chan error, 1)
	go func() {
		_, err := io.WriteString(s.stdin, "tx "+Frame(a.cfg.Source, a.cfg.Target, key)+"\n")
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
	case <-s.done:
		return fmt.Errorf("cec-client stopped while sending: %w", s.failure())
	}
}

func (a *Adapter) Events() <-chan cec.KeyCode {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current == nil {
		return nil
	}
	return a.current.events
}

// Errors reports subprocess, output-reader, and event-buffer failures for the
// current process generation. It is nil before Open and closes with Events.
func (a *Adapter) Errors() <-chan error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current == nil {
		return nil
	}
	return a.current.errors
}

func Frame(source, target uint8, key cec.KeyCode) string {
	header := (source&0x0f)<<4 | (target & 0x0f)
	return fmt.Sprintf("%02x:44:%02x", header, uint8(key))
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
