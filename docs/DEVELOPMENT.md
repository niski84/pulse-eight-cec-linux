# Development

## Build and check the repair

```bash
go build ./...
go test -race ./...
go vet ./...
bash -n scripts/install.sh scripts/reload.sh
python3 -m unittest discover -s scripts -p 'test_*.py' -v
```

The daemon owns the adapter. Do not start a second process that opens it while
the service is running. Leave `P8CEC_DEVICE` unset while testing automatic
discovery.

## Validation environment

Activity-policy tests use simulated button inputs and fake output callbacks.
The daemon subprocess test switches profiles through the real Unix socket,
then feeds another fake remote event and verifies its decision trace without
opening uinput or restarting cec-client. Uinput tests inspect chord press/release
order and cleanup after transient write failures. These checks are distinct
from physical TV/compositor validation.

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
afterward. The daemon's `key` command uses only the Unix socket and injects into
the local keyboard. If the daemon is absent or in shadow mode, it fails without
opening an adapter. `daemon --shadow` still owns the adapter and cannot run
alongside the existing handler. See `SHADOW-VALIDATION.md` for a scheduled test.
