# Distribution

## Release goals

An owner of a Pulse-Eight adapter should be able to install the service without
understanding libCEC internals, while advanced users can build it from source.
Every release must include checksums and a short compatibility matrix.

## Artifact matrix

The public release currently publishes:

- Linux portable tarballs for `amd64` and `arm64`
- Source archive and SHA-256 checksums

Native package formats can be added later without changing the portable
runtime or installer interface.

## Installer behavior

The installer:

1. Checks Linux, Go, systemd, and libCEC prerequisites.
2. Builds the binary and CLI from the tagged source.
3. Installs the user service and vendor-level udev permissions.
4. Reloads udev and systemd user units.
5. Prints the next diagnostic commands.

It must not overwrite an existing desktop input handler without an explicit
confirmation, and it must clearly report when another process owns the adapter.

## GitHub Actions

Tagged releases run:

1. Go build and unit tests
2. Static analysis
3. Cross-builds for `amd64` and `arm64`
4. Checksum generation
5. GitHub release publication

Hardware validation remains a separate self-hosted or manual matrix because the
CEC bus and Pulse-Eight adapter cannot be simulated completely in CI.

## Public project presentation

The README leads with installation, supported behavior, and hardware
limitations. `COMPATIBILITY.md` records tested adapter firmware, distributions,
desktop sessions, TVs, AV receivers, and applications.
