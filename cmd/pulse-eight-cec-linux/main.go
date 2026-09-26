package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
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
		err = runDaemon()
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
	adapter, err := (cecclient.Factory{Config: loadCECConfig()}).NewAdapter(context.Background())
	if err != nil {
		return err
	}
	if err := control.Send(controlSocketPath(), action); err == nil {
		return nil
	}
	session, err := cec.NewSession(adapter, cec.DefaultKeyMap())
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := session.Open(ctx); err != nil {
		return err
	}
	defer session.Close()
	socket, err := control.New(controlSocketPath(), session.SendAction)
	if err != nil {
		return err
	}
	go func() { _ = socket.Run(ctx) }()
	return session.SendAction(ctx, action)
}

func runDaemon() error {
	adapter, err := (cecclient.Factory{Config: loadCECConfig()}).NewAdapter(context.Background())
	if err != nil {
		return err
	}
	session, err := cec.NewSession(adapter, cec.DefaultKeyMap())
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := session.Open(ctx); err != nil {
		return err
	}
	defer session.Close()
	controlServer, err := control.New(controlSocketPath(), session.SendAction)
	if err != nil {
		return err
	}
	controlErrors := make(chan error, 1)
	go func() { controlErrors <- controlServer.Run(ctx) }()
	inputDevice, err := uinput.Open(os.Getenv("P8CEC_UINPUT"))
	if err != nil {
		return err
	}
	defer inputDevice.Close()
	events, ok := adapter.(interface{ Events() <-chan cec.KeyCode })
	if !ok {
		<-ctx.Done()
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-controlErrors:
			if err != nil {
				return fmt.Errorf("control socket: %w", err)
			}
			return nil
		case key, ok := <-events.Events():
			if !ok {
				return errors.New("cec event stream closed")
			}
			if err := inputDevice.SendKeyCode(key); err != nil {
				return err
			}
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
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"binary": cfg.Binary, "binary_available": binaryErr == nil,
		"device": device, "device_available": deviceAvailable,
		"discovery": "libCEC automatic adapter selection",
		"source":    cfg.Source, "target": cfg.Target,
	})
}

func runHTTPServer() error {
	port := envOr("PORT", "9390")
	return serveHTTP(port)
}

func loadCECConfig() cecclient.Config {
	return cecclient.Config{
		Binary: envOr("P8CEC_BINARY", "cec-client"), Device: strings.TrimSpace(os.Getenv("P8CEC_DEVICE")),
		Source: envUint8("P8CEC_SOURCE", 0), Target: envUint8("P8CEC_TARGET", 1),
		Debug: intFromEnv("P8CEC_DEBUG", 8),
	}
}

func controlSocketPath() string {
	return envOr("P8CEC_SOCKET", envOr("XDG_RUNTIME_DIR", "/tmp")+"/pulse-eight-cec.sock")
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

func envUint8(name string, fallback uint8) uint8 {
	value, err := strconv.ParseUint(os.Getenv(name), 0, 8)
	if err != nil {
		return fallback
	}
	return uint8(value)
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
  pulse-eight-cec daemon       Own the adapter and monitor CEC events
  pulse-eight-cec key <action> Send one CEC user-control key
  pulse-eight-cec status       Report local adapter prerequisites
  pulse-eight-cec serve        Run the local health endpoint

Actions include: up, down, left, right, select, play, pause, stop,
volume_up, volume_down, mute, root_menu, exit, previous, next.`)
}
