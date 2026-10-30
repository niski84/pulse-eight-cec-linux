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
	action := parseAction(args[0])
	if action == "" {
		return fmt.Errorf("unknown action %q", args[0])
	}
	if err := control.Send(controlSocketPath(), action); err != nil {
		return fmt.Errorf("send local key through running daemon (no adapter fallback): %w", err)
	}
	return nil
}

func runDaemon(args []string) error {
	flags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	shadow := flags.Bool("shadow", false, "decode and log remote keys without creating a keyboard")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: pulse-eight-cec daemon [--shadow]")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	state := &bridgeState{status: control.Status{Shadow: *shadow}}
	var inputDevice *uinput.Device
	if !*shadow {
		var err error
		inputDevice, err = uinput.Open(os.Getenv("P8CEC_UINPUT"))
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
		return inputDevice.SendAction(action)
	})
	if err != nil {
		return err
	}
	controlServer.WithStatus(state.snapshot)
	controlErrors := make(chan error, 1)
	go func() { controlErrors <- controlServer.Run(ctx) }()
	log.Printf("bridge ready: shadow=%t input_ready=%t socket=%s", *shadow, !*shadow, controlSocketPath())
	var send func(cec.KeyCode) error
	if inputDevice != nil {
		send = inputDevice.SendKeyCode
	}
	err = forwardKeys(ctx, adapter.Events(), adapter.Errors(), controlErrors, send, state)
	cancel()
	return err
}

type bridgeState struct {
	mu     sync.Mutex
	status control.Status
}

func (s *bridgeState) snapshot() control.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// forwardKeys is shared by normal and shadow operation. Only received frames
// enter this path; synthetic CLI keys go directly to the local virtual keyboard.
func forwardKeys(ctx context.Context, events <-chan cec.KeyCode, adapterErrors, controlErrors <-chan error, send func(cec.KeyCode) error, state *bridgeState) error {
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
			if _, supported := uinput.KeyCodeToEvdev(key); !supported {
				state.mu.Lock()
				state.status.Unsupported++
				state.mu.Unlock()
				log.Printf("ignoring unmapped remote key 0x%02x", key)
				continue
			}
			if send == nil {
				log.Printf("shadow: received remote key 0x%02x", key)
				continue
			}
			if err := send(key); err != nil {
				return fmt.Errorf("forward remote key: %w", err)
			}
			state.mu.Lock()
			state.status.Forwarded++
			state.mu.Unlock()
		}
	}
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

func parseAction(value string) cec.Action {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	for action := range cec.DefaultKeyMap() {
		if string(action) == value {
			return action
		}
	}
	return ""
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
  pulse-eight-cec serve        Run the local health endpoint

Actions include: up, down, left, right, select, play, pause, stop,
volume_up, volume_down, mute, root_menu, exit, previous, next.`)
}
