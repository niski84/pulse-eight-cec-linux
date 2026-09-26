# Implementation Plan

## Phase 1 — public foundation

- [x] Scaffold standalone repository
- [x] Add Hermes-style README and install entry point
- [x] Document hardware findings and compatibility targets
- [x] Add systemd and udev packaging layout
- [x] Add tagged-release workflow skeleton
- [x] Add hardware-independent CEC key/lifecycle contract
- [x] Add fake-adapter unit tests

## Phase 2 — generic runtime

- [x] Implement the initial libCEC transport without KDE dependencies
- [x] Keep one adapter owner in the daemon runtime
- [x] Decode incoming CEC user-control events
- [x] Implement Linux `/dev/uinput` output
- [ ] Add suspend/resume and USB hotplug recovery
- [ ] Validate incoming remote frames in shadow mode before enabling uinput
- [ ] Verify receive-to-uinput behavior against the working Plasma handler
- [x] Add Unix-socket control protocol
- [x] Add `pulse-eight-cecctl status` and `key`
- [ ] Add `pulse-eight-cecctl scan`

## Phase 3 — distribution

- [ ] Validate Debian/Ubuntu, Fedora, Arch, and Nix installation
- [x] Build portable `amd64` and `arm64` artifacts
- [ ] Generate `.deb`, `.rpm`, Arch, and Nix packages
- [x] Add checksums to releases
- [ ] Add a real hardware-in-the-loop validation checklist

## Dogfood gate — 2026-09-26

The first live deployment successfully acquired the Pulse-Eight adapter and
transmitted a synthetic pause, but the remote stopped responding. The service
was rolled back to `plasma-cec-debug.service`, restoring remote control.

The next deployment must remain in shadow mode until all of these are proven:

1. Incoming CEC user-control frames are observed from the physical remote.
2. Each observed frame maps to the intended evdev key.
3. Plasma receives the generated uinput event.
4. Adapter disconnect and service restart recover without manual intervention.

## Phase 4 — public release

- [x] Choose final repository owner/name
- [ ] Add project screenshots and HDMI wiring diagram
- [x] Publish compatibility matrix
- [x] Tag `v0.1.0`
- [ ] Share the project with Pulse-Eight hardware maintainers

## Design constraints

- The core must not depend on KDE, Qt, Plasma, Kodi, Plex, or a compositor.
- The service must be the sole owner of the adapter device.
- Local control must use a Unix socket, not an unauthenticated TCP API.
- Hardware-specific behavior must be documented and tested separately.
