# Architecture

## Goals

The service must make the Pulse-Eight adapter useful without requiring KDE,
Plasma Bigscreen, Kodi, or a particular application. It owns the adapter and
publishes normalized input events to the Linux desktop.

## Components

```text
libCEC adapter session
        │
        ▼
CEC event decoder ──► key map ──► uinput keyboard/media device
        │                    └──► optional desktop adapter
        ▼
Unix control socket ◄── pulse-eight-cecctl and local integrations
```

### Adapter session

The adapter session is the only component allowed to open the Pulse-Eight USB
device. The current portable transport owns one long-lived `cec-client` process,
which uses the installed libCEC utility; a direct library binding can replace
that transport without changing the core contract. It owns discovery, logical-
address registration, hotplug recovery, and clean shutdown. Opening the adapter
twice is a known failure mode and is prevented by the session state machine.

### Input output

The default output is `/dev/uinput`. It produces ordinary Linux key events such
as `KEY_UP`, `KEY_ENTER`, `KEY_PLAYPAUSE`, `KEY_VOLUMEUP`, and `KEY_VOLUMEDOWN`.
This is the compatibility layer: Wayland compositors, X11 desktops, media
centers, and desktop applications can consume the same events without knowing
anything about HDMI-CEC.

### Control socket

The Unix socket is local-only and supports key injection through the daemon's
already-open adapter. It is mode `0600`; it must not expose the adapter or
arbitrary shell execution over TCP. Applications that need remote control should
integrate with their own authenticated service and call the local CLI or socket.

## Desktop support

| Environment | Primary path | Fallback |
|---|---|---|
| KDE Plasma / Bigscreen | uinput | desktop-specific integration if needed |
| GNOME Wayland | uinput | none planned initially |
| XFCE / other X11 | uinput | XTest only if explicitly enabled |
| Kodi | uinput | optional Kodi integration later |
| Plex / Plexamp | uinput media keys | application-specific notes |

The core must never import KDE, Qt, Plasma, or a media-center SDK. Those belong
in optional adapters or documentation.

## Lifecycle

The user service starts after the graphical session, claims the adapter, creates
the virtual input device, and retries transient USB/CEC failures with backoff.
After suspend or resume it must reopen the adapter exactly once. A diagnostic
command can temporarily stop the service, but normal applications must never
need to compete for the serial device.

## Reference findings

The first implementation is based on the documented findings in
`/home/nick/goprojects/infrastructure/htpc-hdmi-cec-findings.md`: Pulse-Eight
adapter `2548:1002`, libCEC 6.x, CEC recording-device address `1`, and a
`/dev/uinput` virtual keyboard. Those findings are evidence for the design, not
runtime assumptions that should be hard-coded into the final project.
