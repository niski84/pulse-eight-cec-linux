<div align="center">

# 📺 pulse-eight-cec-linux

### A desktop-neutral Linux input bridge for Pulse-Eight USB HDMI-CEC adapters.

[![Status: prototype](https://img.shields.io/badge/status-prototype-orange.svg?style=flat-square)](docs/ARCHITECTURE.md)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg?style=flat-square)](LICENSE)

</div>

---

Pulse-Eight adapters are excellent hardware, but Linux desktop integration is often
left to a particular desktop shell or media center. This project provides one
small, stable service that turns HDMI-CEC remote events into normal Linux input
events and exposes a local control interface for applications.

The target setup is intentionally ordinary:

```
LG remote → HDMI-CEC → Pulse-Eight USB adapter → libCEC → uinput → Linux desktop/app
```

It is designed for KDE Plasma, GNOME, XFCE, X11, Wayland, Kodi, Plex, and other
applications that understand standard keyboard or media-key events.

## What it provides

- Pulse-Eight USB adapter access with exclusive ownership
- CEC remote-key decoding through libCEC
- Desktop-neutral `/dev/uinput` keyboard/media-key output
- Optional desktop-specific fallbacks where uinput is unavailable
- Local CLI and Unix-socket control for diagnostics and synthetic keys
- User-level systemd service with restart and adapter-lock guidance
- Reproducible portable Linux release artifacts

## Current status

This repository is in the early runtime phase. The reference implementation is
a stable KDE Plasma Bigscreen setup on KDE neon with a Pulse-Eight USB CEC
Adapter v12. The generic daemon owns the adapter, translates CEC events to
uinput, and exposes a protected local control socket. Distribution validation,
recovery handling, and direct libCEC integration remain before the first public
release.

## Install

Release installation will be:

```bash
curl -fsSL https://raw.githubusercontent.com/niski84/pulse-eight-cec-linux/main/scripts/install.sh | bash
```

Until the first release exists, clone the repository and use the development
instructions in `docs/DEVELOPMENT.md`.

## Hardware and permissions

The reference adapter appears as a Pulse-Eight USB device (`2548:1002`) and
usually exposes `/dev/ttyACM0`. Device names are not stable, so the service uses
udev identity rather than assuming a fixed tty path. The installer will explain
the required `dialout` and `input` access and will never silently weaken device
permissions.

Only one process may own the adapter at a time. Do not run `cec-client` while the
service is active; use `pulse-eight-cecctl status` instead. Adapter scanning is
planned but is not yet exposed by the CLI.

## Configuration

The service is intentionally local-only. Configuration covers:

| Setting | Purpose |
|---|---|
| `P8CEC_ADAPTER` | Optional adapter identity or device path |
| `P8CEC_INPUT_BACKEND` | `uinput` by default; optional fallback backends later |
| `P8CEC_SOCKET` | Unix control socket path |
| `P8CEC_LOG_LEVEL` | `error`, `warn`, `info`, or `debug` |
| `P8CEC_OSD_NAME` | CEC device name shown during discovery |

## Troubleshooting

```bash
systemctl --user status pulse-eight-cec
pulse-eight-cecctl status
pulse-eight-cecctl scan
journalctl --user -u pulse-eight-cec -f
```

Common causes of failure are a second process holding the adapter, a stale
post-suspend libCEC connection, missing `uinput` permissions, or an HDMI path
that does not route CEC. See `docs/TROUBLESHOOTING.md` for recovery steps.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Development](docs/DEVELOPMENT.md)
- [Distribution and release plan](docs/DISTRIBUTION.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)
- [CEC key map](docs/KEY-MAP.md)

## Roadmap

- [x] Extract the generic CEC daemon boundary from the KDE reference setup
- [x] Add `uinput` output and Unix-socket control
- [x] Add systemd user installation and udev rules
- [ ] Validate KDE Plasma, GNOME, XFCE, X11, and Wayland
- [ ] Publish `.deb`, `.rpm`, Arch, Nix, and portable tarball artifacts
- [x] Add tagged portable releases and checksums
- [ ] Submit hardware-compatibility notes to Pulse-Eight

## License

MIT. See `LICENSE`.
