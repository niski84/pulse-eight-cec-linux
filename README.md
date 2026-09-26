<div align="center">

# 📺 pulse-eight-cec-linux

### A desktop-neutral Linux input bridge for Pulse-Eight USB HDMI-CEC adapters.

[![Release](https://img.shields.io/github/v/release/niski84/pulse-eight-cec-linux?style=flat-square)](https://github.com/niski84/pulse-eight-cec-linux/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg?style=flat-square)](LICENSE)

</div>

---

Pulse-Eight adapters are excellent hardware, but Linux desktop integration is often
left to a particular desktop shell or media center. This project provides one
small, stable service that turns HDMI-CEC remote events into normal Linux input
events and exposes a local control interface for applications.

The service connects this standard Linux input path:

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

## Status

The first public release provides a user-level daemon, automatic libCEC adapter
selection, Linux uinput output, and a local control socket. It is independent of
KDE, GNOME, Kodi, Plex, X11, and Wayland.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/niski84/pulse-eight-cec-linux/master/scripts/install.sh | bash
```

Or download a portable archive from the [GitHub releases page](https://github.com/niski84/pulse-eight-cec-linux/releases).

## Hardware and permissions

The service asks libCEC to discover the first connected supported adapter. It
does not require a fixed `/dev/ttyACM*` path or product ID. Set `P8CEC_DEVICE`
only when a system has multiple adapters and a specific stable device path is
needed.

Only one process may own the adapter at a time. Do not run `cec-client` while the
service is active; use `pulse-eight-cecctl status` instead. Adapter selection is
delegated to libCEC.

## Configuration

The service is intentionally local-only. Configuration covers:

| Setting | Purpose |
|---|---|
| `P8CEC_DEVICE` | Optional adapter path; unset means automatic discovery |
| `P8CEC_INPUT_BACKEND` | `uinput` by default; optional fallback backends later |
| `P8CEC_SOCKET` | Unix control socket path |
| `P8CEC_LOG_LEVEL` | `error`, `warn`, `info`, or `debug` |
| `P8CEC_OSD_NAME` | CEC device name shown during discovery |

## Troubleshooting

```bash
systemctl --user status pulse-eight-cec
pulse-eight-cecctl status
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

## Releases

Tagged releases publish checksummed portable Linux archives for `amd64` and
`arm64`. Native distribution packages and additional desktop validation can be
added without changing the runtime interface.

## License

MIT. See `LICENSE`.
