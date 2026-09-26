//go:build linux

package uinput

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

const (
	defaultDevicePath = "/dev/uinput"
	bustypeUSB        = 0x03
	uiSetEvbit        = 0x40045564
	uiSetKeybit       = 0x40045565
	uiDevCreate       = 0x5501
	uiDevDestroy      = 0x5502
)

// Open creates a virtual keyboard backed by Linux's uinput device.
func Open(path string) (*Device, error) {
	if path == "" {
		path = defaultDevicePath
	}
	file, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	keys := supportedKeys()
	if err := ioctl(file.Fd(), uiSetEvbit, eventTypeKey); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("enable evdev key events: %w", err)
	}
	for _, key := range keys {
		if err := ioctl(file.Fd(), uiSetKeybit, uintptr(key)); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("enable evdev key %d: %w", key, err)
		}
	}
	if _, err := file.Write(userDeviceDescription()); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write uinput device description: %w", err)
	}
	if err := ioctl(file.Fd(), uiDevCreate, 0); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("create uinput device: %w", err)
	}

	return newDevice(file, func() error {
		return ioctl(file.Fd(), uiDevDestroy, 0)
	}), nil
}

func ioctl(fd uintptr, request uintptr, value uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, value)
	if errno != 0 {
		return errno
	}
	return nil
}

func supportedKeys() []uint16 {
	keys := make([]uint16, 0, len(cecKeyCodes))
	for _, cecKey := range cecKeyCodes {
		key, _ := KeyCodeToEvdev(cecKey)
		keys = append(keys, key)
	}
	return keys
}

func userDeviceDescription() []byte {
	const descriptionSize = 1116
	data := make([]byte, descriptionSize)
	copy(data[:80], "Pulse-Eight CEC Linux")
	binary.LittleEndian.PutUint16(data[80:82], bustypeUSB)
	binary.LittleEndian.PutUint16(data[82:84], 1)
	binary.LittleEndian.PutUint16(data[84:86], 1)
	binary.LittleEndian.PutUint16(data[86:88], 1)
	return data
}
