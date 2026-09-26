# Contributing

## Scope

Keep the core independent of KDE, GNOME, Kodi, Plex, and any single compositor.
Desktop-specific behavior belongs in optional integrations or compatibility
documentation.

## Hardware changes

Do not add a second process that opens the adapter. Use the adapter abstraction
and fake backend for unit tests. Hardware tests must state the adapter identity,
CEC topology, and whether a receiver sits between the adapter and TV.

## Pull requests

Include:

- the user-visible behavior change;
- the tested distributions and desktop sessions;
- `go test ./...` output;
- installer or packaging validation when those paths change; and
- any hardware behavior that remains unverified.

Never include credentials, private HDMI logs, or machine-specific paths in a
pull request.
