# Shadow Validation

The generic daemon must not replace a working desktop CEC handler until its
receive path has been observed with the real remote.

## Procedure

1. Keep the existing desktop handler enabled.
2. Run only the adapter diagnostics after stopping that handler temporarily.
3. Capture raw libCEC traffic while pressing `Up`, `Down`, `OK`, and `Pause`.
4. Confirm each physical press produces a CEC user-control frame (`44`).
5. Start the generic daemon and repeat the same presses.
6. Confirm the daemon reports the decoded key and that `/dev/uinput` receives
   the expected evdev event.
7. Restore the original handler automatically if any check fails.

## Safety rules

- Never run two adapter owners at once.
- Keep a rollback command ready before stopping the working handler.
- Do not treat a healthy process, socket, or synthetic transmit as proof that
  physical remote input works.
- Do not enable the packaged service at boot until receive-to-uinput is proven.

## Current finding

On 2026-09-26 the generic service passed adapter startup, socket creation, and
synthetic pause transmission. Physical remote input was not forwarded, so the
service was disabled and the Plasma handler restored.
