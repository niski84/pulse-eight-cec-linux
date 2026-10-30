# Troubleshooting

## Adapter ownership

Exactly one handler should own the adapter. Do not start this service alongside
Kodi's CEC handler, Plasma's handler, the kernel attachment path, or a separate
cec-client process. Identify the current owner with `fuser -v /dev/ttyACM*`.
Schedule a test before stopping it and retain a rollback command.

The CLI `key` command never opens the adapter. If it reports no responding
daemon, start or inspect the intended daemon instead of launching cec-client.
In normal mode it injects locally; in shadow mode it rejects injection.

## Status and logs

Use `pulse-eight-cecctl status` and `journalctl --user -u pulse-eight-cec -b`.
Live status separates adapter readiness from uinput readiness and reports received,
forwarded, and unsupported keys. An absent daemon gives a nonzero CLI exit code.
HTTP `/api/health` only reports the separate health server's liveness.

## No received keys

Confirm the TV is showing the adapter's HDMI source and CEC is enabled. Start
shadow mode only after the current owner releases the adapter. The daemon needs
libCEC's registered-address NOTICE and ready prompt before it reports startup
success. Automatic discovery is delegated to libCEC; set P8CEC_DEVICE when an
explicit stable adapter path is required.

v0.1.0 incorrectly parsed outgoing traffic. Use the repaired branch for testing;
creating a socket or sending a synthetic command is not evidence of reception.

## Received keys but no application response

In shadow mode zero forwarded keys is expected. In normal mode verify uinput
permissions, the compositor's recognition of the virtual keyboard, and the
application's keyboard shortcuts. Unsupported CEC codes are logged and counted.
Applications such as Plexamp need not offer D-pad navigation even when keyboard
events arrive correctly. Pause and Stop map to KEY_PAUSECD and KEY_STOPCD.

## Restart, USB reconnect, and resume

The systemd unit retries process failures after three seconds. Readiness checks
reject unsuccessful child startup. USB recovery and actual suspend/resume still
need hardware validation. If the process remains alive with a stale adapter
connection, restart this user service once. Automatic stale-connection detection
is not implemented; do not claim that it is.
