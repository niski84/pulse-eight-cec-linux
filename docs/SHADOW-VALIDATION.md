# Hardware validation and rollback

## Recorded failure

On September 26, 2026, the standalone service passed adapter startup, socket
creation, and synthetic CEC transmission. Physical remote input was not
forwarded, so it was disabled and the working Plasma handler restored.

The unreleased repair corrects incoming `>>` versus outgoing `<<` parsing and
adds automated process-level regression coverage. Physical receive-to-desktop
validation remains pending. Do not infer success from a healthy process or a
synthetic key.

## Scheduled test

Do not switch input handlers while someone is using the HTPC. Before testing,
record which handler is active and the exact commands needed to restore it.
On the reference HTPC the last recorded working handler is the user service
`plasma-cec-debug.service`; check its current status before relying on that name.

1. Build/install the repaired binary without enabling its service.
2. Record the active handler and rollback commands. Keep a keyboard or SSH path.
3. Stop the active handler for this test window. Confirm it released the adapter.
4. Run `pulse-eight-cec daemon --shadow` in the foreground. Do not run another
   `cec-client`, Kodi CEC handler, or this daemon's systemd service beside it.
5. Select the TV's HDMI input carrying the adapter. Press real Up, Down, OK,
   and Pause buttons. Confirm logs show decoded keys and `pulse-eight-cecctl
   status` reports increasing `received_keys`, zero `forwarded_keys`,
   `shadow: true`, and `input_ready: false`.
6. Stop shadow mode with Ctrl+C. Run `pulse-eight-cec daemon` in the foreground.
   Use an expendable application and confirm real remote presses reach it.
   Check that `forwarded_keys` increases. `key pause` can separately test the
   local virtual keyboard; it does not prove TV-to-adapter reception.
7. Stop the foreground process. Only after physical input works, test the user
   service, USB disconnect/reconnect, and a separately agreed suspend/resume.
   Unexpected child exits fail the daemon; systemd restarts it after three
   seconds. A live but stale connection may still require an explicit restart.
8. If any step fails, stop this daemon/service and restart the original handler.
   Confirm physical remote operation is restored. Do not enable this service at
   boot until the full test passes.

Record date, revision, raw incoming traffic, observed keys, input/compositor
results, recovery behavior, and rollback outcome. Update `COMPATIBILITY.md` only
with the tests actually observed.
