# Compatibility Matrix

This project separates hardware validation from desktop integration. A row is
only marked working after a real adapter and HDMI path have been tested.

## Tested configuration

| Component | Tested setup | Status |
|---|---|---|
| Adapter | Pulse-Eight USB HDMI-CEC adapter | Tested hardware |
| OS | Ubuntu 24.04-based Linux | Tested host |
| Session | Plasma Bigscreen, Wayland | Tested desktop |
| TV | HDMI-CEC television | Tested display class |
| Output | `/dev/uinput` virtual keyboard | Working reference |

## Target validation

| Environment | Status | Notes |
|---|---|---|
| KDE Plasma Wayland | Validated | uinput input path |
| KDE Plasma X11 | Planned | Validate uinput delivery |
| GNOME Wayland | Planned | Validate compositor media-key handling |
| XFCE X11 | Planned | Validate keyboard navigation and media keys |
| Kodi | Planned | No Kodi dependency in the core |
| Plex | Planned | Validate playback and navigation keys |
| Plexamp | Planned | Document application-specific limitations |

Report new results with adapter firmware, distribution version, desktop
session, HDMI topology, and the exact key behavior observed.
