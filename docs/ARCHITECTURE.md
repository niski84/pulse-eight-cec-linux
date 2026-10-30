# Architecture

## Purpose and prior motivation

The reference HTPC had CEC adapter contention, repeated-open failures, stale
post-suspend connections, and inconsistent portal input delivery. Its working
fix was a patched Plasma handler using uinput. This project attempts to package
the input bridge independently of that KDE implementation.

This motivation does not establish that a new daemon was required. Kernel CEC
input and cec-sync are alternatives whose reference-host behavior has not been
compared. The project reuses libCEC rather than reimplementing the USB protocol.

## Current data path

`remote → CEC adapter → libCEC cec-client → inbound frame filter → key map → uinput → application`

The transport runs one long-lived `cec-client -t r` process. It requests NOTICE
and TRAFFIC output, waits for registered logical addresses plus the ready prompt,
and only emits complete incoming `TRAFFIC ... >> ...:44:...` presses directed to
those registered addresses. Transmitted `<<` frames, debug descriptions, other
CEC destinations, and malformed frames are ignored. This prevents self-echo and
duplicate decoding. Each supported press produces one keyboard press/release;
continuous held-key state is not currently preserved.

Process generations own their channels and child lifecycle. Startup has a
30-second bound; shutdown has a two-second grace period followed by a process
group kill. Subprocess, scanner, and overflow errors are surfaced. Unknown CEC
keys are logged and counted without crashing the input service.

## Output and local control

Normal mode creates a Linux uinput virtual keyboard. Shadow mode uses the same
receive path but logs keys without opening uinput. Shadow mode still owns the
adapter exclusively.

The private Unix socket provides `key` and `status`. `key` targets the local
virtual keyboard, not the CEC bus. The CLI never opens an adapter as a fallback.
Status reports the running bridge's readiness and counters, not hardware
validation. The socket is mode `0600`; an exclusive lock protects endpoint
ownership and stale-socket recovery.

The optional HTTP server is a separate development liveness service. Both the
scaffold mux and command entrypoint use the same handler, bound to loopback.
Its health response explicitly says hardware was not checked.

## Recovery boundary

The user service restarts failed processes after three seconds, including an
unexpectedly exited `cec-client`. This is distinct from detecting a live but
stale connection after resume. Automatic detection/recovery of that condition
is not implemented or claimed. Real hotplug and sleep tests remain pending.

Applications must support the emitted keys. Compositor policies and application
shortcuts still apply; uinput does not add D-pad navigation to an app that lacks
it. No KDE, Qt, Kodi, or Plex SDK is part of the core.
