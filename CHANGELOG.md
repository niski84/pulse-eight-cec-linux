# Changes

## Unreleased activity profiles — 2026-10-30

Add explicit, switchable activity profiles with key chords and suppression,
shared by remote and synthetic CLI input. Add a private-socket decision trace
and hardware-free `explain` preview. Configuration is validated before devices
open, and profile changes do not reopen the adapter. Chords release keys in
reverse order and attempt cleanup after output errors.

This is an application-level policy feature, not a new CEC driver. It could
operate above a future kernel backend. Automatic app detection, gestures,
player-specific routing, and kernel-input support remain proposals. Real
remote/compositor validation is still pending.

## Unreleased repair — 2026-10-29

Fix received TV-remote input being ignored because v0.1.0 decoded outgoing CEC
traffic. Only received presses directed to registered local addresses are now
forwarded. Startup waits for libCEC readiness and failures are surfaced; shutdown
is bounded even when a subprocess or its descendants stop responding.

Synthetic CLI keys now target the local keyboard through the running daemon.
They never open a competing adapter or impersonate the TV on the CEC bus. Live
status and shadow mode separate reception, keyboard injection, and process
liveness. Unsupported keys no longer terminate the daemon; media Pause/Stop
use the appropriate Linux event codes.

Installation now leaves the service stopped, respects custom binary paths,
builds source from its module directory, and supports the repaired portable
archive layout. Archives include the service, udev rule, CLI alias, installer,
and documentation. Compatibility statements now reflect the failed standalone
trial rather than the separate working Plasma handler.

Automated regression and installer tests pass. Physical remote input,
application delivery, USB recovery, and suspend/resume remain unverified. No
automatic recovery of a live-but-stale post-resume connection is claimed.

## v0.1.0 — 2026-09-26

Initial experimental public implementation and Linux amd64/arm64 archives.
Known receive-direction defect; physical remote validation failed. Use the
repair branch for further testing rather than treating v0.1.0 as validated.
