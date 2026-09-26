# Distribution and Release Plan

## Release goals

An owner of a Pulse-Eight adapter should be able to install the service without
understanding libCEC internals, while advanced users can build it from source.
Every release must include checksums and a short compatibility matrix.

## Artifact matrix

The first GitHub release should publish:

- Linux portable tarballs for `amd64` and `arm64`
- Debian packages for common Debian/Ubuntu systems
- RPM packages for Fedora/openSUSE-compatible systems
- An Arch package recipe
- A Nix flake and package expression
- Source archive and SHA-256 checksums

Package-manager repositories can follow after the first stable release. The
portable archive is the fallback and the installer should prefer a native
package when the host distribution is recognized.

## Installer behavior

The installer will:

1. Detect architecture, init system, desktop session, and distribution.
2. Check for libCEC and supported Pulse-Eight hardware access.
3. Install the binary, CLI, udev rule, and user service.
4. Add the current user to only the groups required by the selected backend.
5. Reload udev and systemd user units.
6. Run a non-destructive adapter health check.
7. Print the exact next diagnostic commands.

It must not overwrite an existing desktop input handler without an explicit
confirmation, and it must clearly report when another process owns the adapter.

## GitHub Actions

Tagged releases will run:

1. Go/C++ build and unit tests
2. Static analysis and shell validation
3. Linux integration tests using a fake adapter boundary
4. Cross-builds for `amd64` and `arm64`
5. Package generation
6. Checksum and SBOM generation
7. GitHub release publication

Hardware validation remains a separate self-hosted or manual matrix because the
CEC bus and Pulse-Eight adapter cannot be simulated completely in CI.

## Public project presentation

The README will lead with the problem, supported setup, one-line install, and
hardware limitations. It will include a small wiring diagram and a “known good”
reference configuration so Pulse-Eight can evaluate the project quickly. A
`COMPATIBILITY.md` file will record tested adapter firmware, distributions,
desktop sessions, TVs, AV receivers, and applications.
