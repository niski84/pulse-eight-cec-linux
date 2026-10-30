# Distribution

## Published versus repaired

The published v0.1.0 archives predate the receive-path repair and failed physical
remote validation. The repaired branch is not a hardware-validated release.
Publish a release candidate for a scheduled hardware trial; record the result
before promoting a stable compatibility claim. Tags containing a hyphen are
marked prereleases by the workflow.

## Archive contents

The repaired release workflow builds statically linked Go Linux binaries for
amd64 and arm64. Each archive includes the CLI symlink, installer, user service,
udev rule, documentation, compatibility statement, and license. SHA256SUMS
covers the archives. Go is not needed to install an extracted binary archive;
libCEC/cec-client and a systemd user session are still required.

The source installer builds from the module directory regardless of the caller's
current directory. Both installation paths leave the service stopped, retain
existing boot enablement, and print permission/test instructions. An already
running bridge must be stopped before upgrading. Custom binary directories are
reflected in the installed unit.

## Checks and hardware gate

CI runs race-enabled Go tests, vet, shell syntax checks, and installer tests
using disposable directories and mocked host tools. Portable cross-builds and
checksums are separate from runtime validation. Distribution-specific native
packages, clean-distro installation, physical input, hotplug, and suspend/resume
are not yet validated. See `SHADOW-VALIDATION.md`.
