# Compatibility

## Evidence boundary

The standalone daemon is **not yet validated with a physical TV remote**.
Automated subprocess tests establish frame parsing, lifecycle behavior, local
control, shadow operation, and key mapping. They do not establish HDMI routing,
compositor delivery, adapter recovery, or usability in a particular application.

## Reference hardware

The existing HTPC uses a Pulse-Eight USB adapter (USB `2548:1002`, firmware 12),
libCEC 6.0.2, KDE neon on Ubuntu 24.04, Plasma Bigscreen/Wayland, and an LG TV.
Remote navigation worked through a separately patched Plasma input handler and
its uinput backend. That is evidence for the hardware path and the Plasma
handler, not for this standalone daemon.

On September 26, 2026, this standalone daemon opened the adapter and accepted a
synthetic CEC transmission, but physical remote input was not forwarded. It was
disabled and the Plasma handler restored. The unreleased repair corrects the
receive-direction parser and must repeat the hardware validation.

## Pending validation

- Reference HTPC: physical Up, Down, Left, Right, Select, and media keys to uinput.
- Reference HTPC: compositor delivery and foreground application behavior.
- USB disconnect/reconnect and service restart without duplicate ownership.
- Actual suspend/resume, including a child connection that stays alive but stale.
- KDE X11, GNOME Wayland, XFCE X11, and other distributions.
- Kodi and Plex application behavior; avoid their own CEC ownership conflicts.
- Plexamp media shortcuts; its UI need not respond to D-pad navigation.
- Real `arm64` hardware. Cross-compilation alone is not runtime validation.

Report results with the software revision, adapter firmware, OS/session, TV/AVR
models, HDMI topology, and exact behavior. Follow [the test and rollback
procedure](docs/SHADOW-VALIDATION.md) before replacing a working handler.
