# Compatibility Matrix

This project separates hardware validation from desktop integration. A row is
only marked working after a real adapter and HDMI path have been tested.

## Reference configuration

| Component | Tested setup | Status |
|---|---|---|
| Adapter | Pulse-Eight USB CEC Adapter v12 (`2548:1002`) | Working reference |
| OS | KDE neon / Ubuntu 24.04 base | Working reference |
| Session | Plasma Bigscreen, Wayland | Working reference |
| TV | LG TV | Working reference |
| Output | `/dev/uinput` virtual keyboard | Working reference |

## Target validation

| Environment | Status | Notes |
|---|---|---|
| KDE Plasma Wayland | Reference validation | Extracted from HTPC implementation |
| KDE Plasma X11 | Planned | Validate uinput delivery |
| GNOME Wayland | Planned | Validate compositor media-key handling |
| XFCE X11 | Planned | Validate keyboard navigation and media keys |
| Kodi | Planned | No Kodi dependency in the core |
| Plex | Planned | Validate playback and navigation keys |
| Plexamp | Planned | Document application-specific limitations |

Report new results with adapter firmware, distribution version, desktop
session, HDMI topology, and the exact key behavior observed.
