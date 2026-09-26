# Development

## Build the scaffold

```bash
go build ./...
go test ./...
go vet ./...
```

The daemon owns the adapter. Do not start a second process that opens it while
the service is running. Leave `P8CEC_DEVICE` unset while testing automatic
discovery.

## Validation environment

Validation uses a Linux desktop with libCEC, a Pulse-Eight USB HDMI-CEC
adapter, an HDMI-CEC display path, and `/dev/uinput`. Device paths, USB product
identifiers, and credentials are not runtime requirements.

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
