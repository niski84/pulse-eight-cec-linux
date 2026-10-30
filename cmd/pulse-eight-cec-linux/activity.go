package main

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"

	"github.com/niski84/pulse-eight-cec-linux/internal/control"
	"github.com/niski84/pulse-eight-cec-linux/internal/policy"
)

func loadPolicy(path string) (*policy.Engine, error) {
	if path == "" {
		return policy.NewDefault(), nil
	}
	return policy.LoadFile(path)
}

func runProfiles(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: pulse-eight-cec profiles")
	}
	names, err := control.ReadProfiles(controlSocketPath())
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(names)
}

func runProfile(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: pulse-eight-cec profile [name]")
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
		if strings.TrimSpace(name) == "" {
			return errors.New("profile name must not be blank")
		}
	}
	active, err := control.SelectProfile(controlSocketPath(), name)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"active_profile": active})
}

func runTrace(args []string) error {
	flags := flag.NewFlagSet("trace", flag.ContinueOnError)
	limit := flags.Int("limit", 20, "recent decisions to return (0 = all, maximum 50)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *limit < 0 || *limit > 50 {
		return errors.New("usage: pulse-eight-cec trace [--limit 0..50]")
	}
	trace, err := control.ReadTrace(controlSocketPath(), *limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(trace)
}

func runExplain(args []string) error {
	flags := flag.NewFlagSet("explain", flag.ContinueOnError)
	config := flags.String("config", os.Getenv("P8CEC_CONFIG"), "activity profile JSON configuration")
	profile := flags.String("profile", "", "activity to preview (defaults to configured selection)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: pulse-eight-cec explain [--config path] [--profile name] <input>")
	}
	key, err := policy.ParseInput(normalizeInput(flags.Arg(0)))
	if err != nil {
		return err
	}
	engine, err := loadPolicy(*config)
	if err != nil {
		return err
	}
	decision, err := engine.Explain(uint8(key), *profile)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(decision)
}
