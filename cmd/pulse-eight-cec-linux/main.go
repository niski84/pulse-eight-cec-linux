package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/niski84/pulse-eight-cec-linux/internal/cec"
	"github.com/niski84/pulse-eight-cec-linux/internal/cecclient"
	"github.com/niski84/pulse-eight-cec-linux/internal/control"
	"github.com/niski84/pulse-eight-cec-linux/internal/policy"
	"github.com/niski84/pulse-eight-cec-linux/internal/uinput"
)

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "key":
		err = runKey(os.Args[2:])
	case "daemon":
		err = runDaemon(os.Args[2:])
	case "status":
		err = runStatus()
	case "profiles":
		err = runProfiles(os.Args[2:])
	case "profile":
		err = runProfile(os.Args[2:])
	case "trace":
		err = runTrace(os.Args[2:])
	case "explain":
		err = runExplain(os.Args[2:])
	case "serve":
		err = runHTTPServer()
	case "help", "-h", "--help":
		printUsage()
	default:
		printUsage()
		err = fmt.Errorf("unknown command %q", command)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pulse-eight-cec:", err)
		os.Exit(1)
	}
}

func runKey(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: pulse-eight-cec key <action>")
	}
	input := normalizeInput(args[0])
	if _, err := policy.ParseInput(input); err != nil {
		return err
	}
	if err := control.Send(controlSocketPath(), cec.Action(input)); err != nil {
		return fmt.Errorf("send local key through running daemon (no adapter fallback): %w", err)
	}
	return nil
}

func runDaemon(args []string) error {
	flags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	shadow := flags.Bool("shadow", false, "decode and log remote keys without creating a keyboard")
	config := flags.String("config", os.Getenv("P8CEC_CONFIG"), "activity profile JSON configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: pulse-eight-cec daemon [--shadow] [--config path]")
	}
	engine, err := loadPolicy(*config)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	state := &bridgeState{status: control.Status{Shadow: *shadow}, policy: engine}
	var inputDevice *uinput.Device
	if !*shadow {
		var err error
		inputDevice, err = uinput.OpenWithKeys(os.Getenv("P8CEC_UINPUT"), engine.RequiredKeys())
		if err != nil {
			return err
		}
		defer func() {
			if err := inputDevice.Close(); err != nil {
				log.Printf("close keyboard: %v", err)
			}
		}()
		state.status.InputReady = true
	}
	adapter := cecclient.New(loadCECConfig())
	if err := adapter.Open(ctx); err != nil {
		return err
	}
	defer func() {
		if err := adapter.Close(); err != nil {
			log.Printf("close adapter: %v", err)
		}
	}()
	state.status.AdapterReady = true
	controlServer, err := control.New(controlSocketPath(), func(ctx context.Context, action cec.Action) error {
		if *shadow {
			return errors.New("shadow mode does not inject keys")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return dispatchCLI(engine, action, inputDevice.SendChord)
	})
	if err != nil {
		return err
	}
	controlServer.WithStatus(state.snapshot)
	controlServer.WithPolicy(engine)
	controlErrors := make(chan error, 1)
	go func() { controlErrors <- controlServer.Run(ctx) }()
	log.Printf("bridge ready: shadow=%t input_ready=%t socket=%s", *shadow, !*shadow, controlSocketPath())
	var send func([]uint16) error
	if inputDevice != nil {
		send = inputDevice.SendChord
	}
	err = forwardKeys(ctx, adapter.Events(), adapter.Errors(), controlErrors, send, state)
	cancel()
	return err
}

type bridgeState struct {
	mu     sync.Mutex
	status control.Status
	policy *policy.Engine
}

func (s *bridgeState) snapshot() control.Status {
	s.mu.Lock()
	status := s.status
	s.mu.Unlock()
	if s.policy != nil {
		status.ActiveProfile = s.policy.ActiveProfile()
	}
	return status
}

// forwardKeys is shared by normal and shadow operation. Remote and CLI inputs
// share the policy engine; only received frames increment the remote counters.
func forwardKeys(ctx context.Context, events <-chan cec.KeyCode, adapterErrors, controlErrors <-chan error, send func([]uint16) error, state *bridgeState) error {
	engine := state.policy
	if engine == nil {
		engine = policy.NewDefault()
	}
	state.mu.Lock()
	shadow := state.status.Shadow
	state.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-controlErrors:
			if err != nil {
				return fmt.Errorf("control socket: %w", err)
			}
			return nil
		case err, ok := <-adapterErrors:
			if !ok {
				adapterErrors = nil
				continue
			}
			if err != nil {
				return fmt.Errorf("adapter: %w", err)
			}
		case key, ok := <-events:
			if !ok {
				return errors.New("cec event stream closed")
			}
			state.mu.Lock()
			state.status.Received++
			state.mu.Unlock()
			decision, err := engine.Dispatch(uint8(key), "remote", shadow, send)
			if err != nil {
				return fmt.Errorf("forward remote input=%s profile=%s: %w", decision.Input, decision.Profile, err)
			}
			switch decision.Outcome {
			case policy.Unmapped:
				state.mu.Lock()
				state.status.Unsupported++
				state.mu.Unlock()
				log.Printf("ignoring unmapped remote key 0x%02x", key)
			case policy.Ignored:
				state.mu.Lock()
				state.status.Ignored++
				state.mu.Unlock()
			case policy.Shadow:
				log.Printf("shadow: input=%s profile=%s reason=%s", decision.Input, decision.Profile, decision.Reason)
			case policy.Forwarded:
				state.mu.Lock()
				state.status.Forwarded++
				state.mu.Unlock()
			}
		}
	}
}

func dispatchCLI(engine *policy.Engine, action cec.Action, send func([]uint16) error) error {
	key, err := policy.ParseInput(normalizeInput(string(action)))
	if err != nil {
		return err
	}
	decision, err := engine.Dispatch(uint8(key), "cli", false, send)
	if err != nil {
		return err
	}
	if decision.Outcome == policy.Unmapped {
		return fmt.Errorf("unmapped input %s", decision.Input)
	}
	return nil
}

func runStatus() error {
	cfg := loadCECConfig()
	_, binaryErr := exec.LookPath(cfg.Binary)
	deviceAvailable := any(nil)
	device := cfg.Device
	if device == "" {
		device = "auto"
	} else {
		_, deviceErr := os.Stat(cfg.Device)
		deviceAvailable = deviceErr == nil
	}
	status, daemonErr := control.ReadStatus(controlSocketPath())
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{
		"binary": cfg.Binary, "binary_available": binaryErr == nil,
		"device": device, "device_available": deviceAvailable,
		"discovery":      "libCEC automatic adapter selection",
		"daemon_running": daemonErr == nil, "daemon": status,
	}); err != nil {
		return err
	}
	if daemonErr != nil {
		return fmt.Errorf("daemon is not responding: %w", daemonErr)
	}
	return nil
}

func runHTTPServer() error {
	port := envOr("PORT", "9390")
	return serveHTTP(port)
}

func loadCECConfig() cecclient.Config {
	return cecclient.Config{
		Binary: envOr("P8CEC_BINARY", "cec-client"), Device: strings.TrimSpace(os.Getenv("P8CEC_DEVICE")),
		Debug: intFromEnv("P8CEC_DEBUG", 8),
	}
}

func controlSocketPath() string {
	runtimeDir := envOr("XDG_RUNTIME_DIR", filepath.Join(os.TempDir(), "pulse-eight-cec-"+strconv.Itoa(os.Getuid())))
	return envOr("P8CEC_SOCKET", filepath.Join(runtimeDir, "pulse-eight-cec.sock"))
}

func normalizeInput(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intFromEnv(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func printUsage() {
	fmt.Println(`pulse-eight-cec — Linux Pulse-Eight HDMI-CEC bridge

Usage:
  pulse-eight-cec daemon       Forward received remote keys to a Linux keyboard
  pulse-eight-cec daemon --shadow  Log received keys without injecting input
  pulse-eight-cec key <action> Inject a local key through the running daemon
  pulse-eight-cec status       Report live daemon state and local prerequisites
  pulse-eight-cec profiles     List configured activities in the running daemon
  pulse-eight-cec profile [name]  Query or select an activity without reopening CEC
  pulse-eight-cec trace [--limit 20]  Show recent input decisions
  pulse-eight-cec explain [--config path] [--profile name] <input>  Preview offline
  pulse-eight-cec serve        Run the local health endpoint

Actions include: up, down, left, right, select, play, pause, stop,
volume_up, volume_down, mute, root_menu, exit, previous, next.`)
}
