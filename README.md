# pulse-eight-cec-linux

Use a TV remote as a Linux keyboard through a Pulse-Eight USB HDMI-CEC adapter.
The service is intended for navigation and media keys across desktop applications,
without requiring Kodi or Plasma Bigscreen.

## Status: experimental, hardware validation pending

The published **v0.1.0** has a receive-direction bug: it parses transmitted CEC
traffic instead of received remote presses. Its recorded hardware trial failed
and the previously working Plasma handler was restored. Do not treat that release
as a validated replacement for an existing input handler.

The unreleased repair adds inbound parsing, startup and shutdown checks, local
keyboard injection, live status, and a shadow mode. Automated process and installer
tests pass; physical remote-to-desktop, hotplug, and suspend/resume validation are
still required. See [compatibility](COMPATIBILITY.md) and the
[hardware test procedure](docs/SHADOW-VALIDATION.md).

## How it fits

`TV remote → HDMI-CEC → Pulse-Eight adapter → libCEC/cec-client → this service → Linux uinput → desktop/application`

We reuse Pulse-Eight's Linux software to communicate with the adapter. Our code
supplies a small input mapper, virtual keyboard, user service, and local control
socket. An application must support the resulting keyboard/media keys; this
cannot make a mouse-only interface navigable by D-pad.

Existing alternatives include:

- [Kodi's built-in libCEC integration](https://libcec.pulse-eight.com/FAQ), for controlling Kodi.
- [Linux's CEC subsystem](https://docs.kernel.org/admin-guide/media/cec.html), using `pulse8-cec`, `inputattach`, and `cec-ctl` as an alternative adapter/input path.
- [cec-sync](https://gitlab.com/kira-bruneau/cec-sync), for Linux home-theater integration, including documented Steam Deck input support.

These alternatives have not been compared on the reference HTPC. This project
does not claim to be the first or only system-wide Linux CEC solution.

## Install without starting the service

Requirements: Linux, systemd user services, and libCEC's `cec-client` (typically
provided by `cec-utils`). A source build also needs Go 1.23 or later.

For the unreleased repair, use its source checkout:

```bash
bash scripts/install.sh
```

Run the installer as your normal desktop user. It installs the binary, CLI alias,
and user service, but does not start it, change boot enablement, stop another
handler, or alter device permissions. If this service is already running, the
installer asks you to stop it before replacing the binary.

Future portable archives containing this repair include the same installer and
service/udev files. After extracting one, run `bash scripts/install.sh` from the
archive. Go is not required for the binary archive. The old v0.1.0 archives do not
contain this repaired packaging.

The installer prints the optional administrator command for installing the
permission rule. Confirm access to the adapter and `/dev/uinput`; new group
membership may require a new login. Do not run the bridge as root.

## Validate before enabling

Only one process may own the adapter. Schedule a test window, record the current
handler and its rollback command, and stop that handler before running:

```bash
pulse-eight-cec daemon --shadow
```

Shadow mode claims the adapter and logs decoded keys, but never opens uinput or
injects keys. It is not a passive observer that can run beside another handler.
In another terminal:

```bash
pulse-eight-cecctl status
```

Press real remote buttons and confirm the receive counter changes. Then follow
the [hardware test procedure](docs/SHADOW-VALIDATION.md) for normal input and
recovery tests. Enable at boot only after that validation:

```bash
systemctl --user enable --now pulse-eight-cec.service
```

## Local controls

```bash
pulse-eight-cecctl status
pulse-eight-cecctl key pause
journalctl --user -u pulse-eight-cec -f
```

`status` queries the running daemon and reports adapter/input readiness, shadow
mode, and received/forwarded/unmapped key counts. It exits unsuccessfully when no
daemon responds. Installed prerequisites are reported separately.

`key` injects a key into the daemon's local virtual keyboard. It does not send a
CEC command to the TV and never opens a second adapter. It fails when the daemon
is unavailable or in shadow mode. A synthetic key does not validate TV reception.

The optional `serve` command is only the development health server. Its HTTP
liveness response does not indicate that the adapter or keyboard is working.

## Configuration

- `P8CEC_DEVICE`: optional adapter path; unset uses libCEC discovery.
- `P8CEC_BINARY`: path to `cec-client`.
- `P8CEC_DEBUG`: libCEC log mask; NOTICE and TRAFFIC are always enabled for parsing.
- `P8CEC_SOCKET`: local Unix socket; defaults to the user runtime directory.
- `P8CEC_UINPUT`: virtual-input device path, normally `/dev/uinput`.

Use `systemctl --user edit pulse-eight-cec` to set service environment overrides.
The control socket is local-only and mode `0600`. Adapter discovery is delegated
to libCEC; a stable explicit path is useful when several adapters are attached.

## Documentation and license

[Architecture](docs/ARCHITECTURE.md) · [Development](docs/DEVELOPMENT.md) ·
[Distribution](docs/DISTRIBUTION.md) · [Troubleshooting](docs/TROUBLESHOOTING.md) ·
[Key map](docs/KEY-MAP.md) · [Compatibility](COMPATIBILITY.md)

MIT; see [LICENSE](LICENSE). GitHub releases provide checksummed Linux `amd64`
and `arm64` archives. Architecture builds are not hardware compatibility tests.
