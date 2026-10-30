//go:build !linux

package uinput

import (
	"errors"
)

// Open is unavailable on non-Linux systems because uinput is Linux-specific.
func Open(string) (*Device, error) {
	return nil, errors.New("uinput: Linux is required")
}

func OpenWithKeys(string, []uint16) (*Device, error) {
	return nil, errors.New("uinput: Linux is required")
}
