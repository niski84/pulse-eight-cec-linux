# Development

## Build the scaffold

```bash
go build ./...
go test ./...
go vet ./...
```

The daemon and CLI are intentionally being extracted in stages. Do not add a
second process that opens `/dev/ttyACM0`; adapter ownership belongs to the core
service.

## Reference machine

The current validation machine is KDE neon on an x86-64 HTPC with:

- Pulse-Eight USB CEC Adapter v12
- USB identity `2548:1002`
- libCEC 6.x
- HDMI CEC routed through an LG TV
- Plasma Bigscreen on Wayland
- `/dev/uinput` input injection

Use the reference findings in the workspace infrastructure notes, but keep
machine-specific paths and credentials out of this repository.

## Testing strategy

- Unit-test CEC frame decoding and key mapping without hardware.
- Test adapter lifecycle with a fake libCEC boundary.
- Test uinput event generation against a disposable virtual device.
- Test installer idempotence in clean Debian and Fedora containers.
- Run manual HDMI validation before each compatibility release.

## Safety

Do not run a second `cec-client` while the service owns the adapter. If direct
bus inspection is necessary, stop the user service first and restart it
afterward. The daemon's `key` command uses the Unix socket when the daemon is
running, so it does not compete for the adapter.
