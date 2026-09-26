# Troubleshooting

## Adapter is busy

```bash
fuser -v /dev/ttyACM0
systemctl --user status pulse-eight-cec
```

Only one process may own the Pulse-Eight adapter. Stop competing diagnostic
tools or desktop handlers before starting this service.

## No remote events

Confirm the TV is displaying the HDMI input connected through the adapter, then
check:

```bash
pulse-eight-cecctl status
journalctl --user -u pulse-eight-cec -b
```

CEC routing depends on the TV, receiver, cable path, and active source state.

## Input device exists but apps do not respond

Check that the service created a uinput device and that the compositor is
running. The uinput path is preferred because it works across Wayland and X11.
Application-specific limitations should be recorded in `COMPATIBILITY.md`, not
patched into the core daemon.

## Resume or hotplug failure

Restart the user service once and inspect the journal. The service should reopen
the adapter exactly once and report the resulting logical address. Repeated
opens are a bug and should be reported with the complete startup log.
