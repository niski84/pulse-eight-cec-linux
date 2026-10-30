# Why an application-level control service?

We started with unreliable remote input when changing applications on a Linux
HTPC: adapter contention, repeated opens, and inconsistent input delivery. The
initial goal was a desktop-independent input bridge using existing libCEC
support. That history explains the work, but does not establish that a custom
adapter path is superior to Linux's native CEC input subsystem.

The more useful direction is a control layer that gives a limited TV remote
different meanings for different activities and explains the decisions it makes.
Its value should survive a change of adapter backend.

## What the native kernel path already does

The kernel CEC infrastructure uses rc-core and supports remapping scancodes into
Linux input keys. Its tools provide device configuration and diagnostics. The
kernel documentation also describes adapter attachment and suspend integration.
System-wide input, key remapping, or an automatic service are therefore not
exclusive features of this project.

Sources: [CEC protocol and remapping](https://www.kernel.org/doc/html/next/userspace-api/media/rc/rc-protos.html#cec-rc-proto-cec),
[kernel CEC setup](https://docs.kernel.org/admin-guide/media/cec.html).

The kernel driver does not itself choose a browser activity, resolve a remote
button to an application shortcut in that activity, and explain which policy
made that choice. Software above the kernel can add those behaviors too. We
should compare the complete user experience, not claim that the kernel makes
them impossible.

General-purpose tools such as [Input Remapper](https://github.com/sezanzeb/input-remapper)
already offer combinations and programmable macros on X11 and Wayland. Activity
bindings are not novel by themselves. The proposed distinction is a coherent
CEC input, activity, and diagnostic workflow; whether that is easier than an
existing combination of tools still needs user testing.

## Implemented in the activity-profile feature branch

### Explicit activity profiles

The same incoming button can resolve differently under `desktop`, `browser`, or
another configured activity. Bindings support one key, a chord of up to eight
distinct keys, or an explicit ignore action. Profile selection and action output
are serialized so changing a profile takes effect between actions. It does not
restart the adapter, force window focus, or infer the active app.

Without a configuration file, the built-in desktop profile preserves the prior
default mappings. Every configured profile inherits those defaults and overrides
only specified inputs. The bundled example changes Back to Alt+Left for browser
navigation, maps presentation buttons to PageUp/PageDown/Space, and suppresses
the Power button in those profiles. Examples are not hardware compatibility
claims or guaranteed shortcuts for every application.

### Explainable decisions

`trace` reports the last 50 input decisions in memory: sequence, UTC timestamp,
remote/CLI origin, selected profile, explicit or default mapping, output keys,
outcome, and reason. Ignored and unsupported inputs are distinct from emitted
keys. The history resets on restart and is not a raw CEC bus recording.

Successful output means the keyboard event write completed; it does not prove
that the compositor or application responded. Output failures include the input
and profile in the daemon error log. The service observes its CEC inputs and
synthetic commands, not ordinary typing from other keyboards.

### Offline preview

`explain --config examples/profiles.json --profile browser exit` shows a proposed
decision without opening CEC, uinput, or the control socket. It does not mutate a
running daemon, emit input, or append a trace record. This lets users check
configuration without interrupting a working HTPC.

## Configuration

Pass `--config path` to `daemon` or set `P8CEC_CONFIG`. Configuration is validated
before opening either device. `examples/profiles.json` is a complete example.

- Top-level `default_profile` selects one entry in `profiles`.
- Each profile contains optional `bindings` keyed by an existing CEC action name
  such as `exit`, or a raw User Control Pressed operand such as `0x72`.
- A binding has either `keys` with named Linux keycodes, or `ignore: true`.
- Chords use Linux names such as `KEY_LEFTALT` and `KEY_LEFT`. Modifiers are
  pressed first; all keys are released in reverse order. Chords are simultaneous
  combinations, not timed macros or sequences. Keyboard layout and app shortcuts
  still determine the resulting behavior.
- Common letters, numbers, F1–F12, navigation, media keys, and Ctrl/Alt/Shift/Meta
  modifiers are supported. `KEY_PAUSECD`/`KEY_STOPCD` are media commands;
  `KEY_PAUSE`/`KEY_STOP` retain their distinct Linux meanings.
- Unknown fields, invalid names, duplicate JSON keys, duplicate input aliases,
  invalid chords, and oversized files fail explicitly.

Use `profiles` to list activities and `profile NAME` to select one through the
existing private Unix socket. Selection is per-daemon session; restart restores
the configured default. Changing the file itself requires a scheduled daemon
restart; hot-reload is not implemented. Synthetic `key` commands follow the same
selected activity as physical presses. Shadow mode records proposed decisions
and never creates a keyboard or accepts synthetic injection.

## Candidate extensions, not implemented

1. **Choose the playback target.** Pause the selected player even when a browser
   or launcher has focus. MPRIS provides player methods and capability flags;
   other services, including cec-sync, already use MPRIS. The useful experiment
   is explicit target selection and predictable fallback, not claiming MPRIS is
   new. Non-MPRIS app integrations should use that application's owning service.
2. **Gestures for limited remotes.** Give tap, hold, or double-tap different
   meanings per activity, with configurable repeat filtering. This requires real
   press/release/timing data; the current press-only transport cannot reliably
   distinguish those gestures. Preserve immediate navigation unless opted in.
3. **Explain recovery and bus changes.** Correlate disconnect, registration,
   source switching, and output failures with input decisions. Recover from
   evidenced failures without treating a quiet remote as proof of a fault.
   A live-but-stale post-resume connection remains unresolved.
4. **Reproduce an input incident offline.** Replay a recorded normalized-input
   fixture into a fake output, comparing profile decisions without changing the
   real desktop. Raw bus capture and deterministic replay are not yet implemented.
5. **Add a kernel input backend.** Feed normalized remote events into the same
   profile/trace engine, with careful input ownership to avoid duplicate keys.
   Compare it with the libCEC backend on the reference hardware. Do not replace
   the working handler while someone is using the HTPC.

Sources: [MPRIS player interface](https://specifications.freedesktop.org/mpris/latest/Player_Interface.html),
[cec-sync implemented/planned features](https://gitlab.com/kira-bruneau/cec-sync/-/raw/main/README.md).

## What would justify continuing?

A user should be able to configure a useful activity, preview it, switch to it,
and diagnose a received-but-unacted-on button more easily than assembling the
equivalent pieces themselves. That is a product hypothesis, not a measured
advantage yet. First validate baseline input and compare complete setups on the
same hardware. Keep whichever backend is more reliable; keep custom policy only
where it solves an observed problem.
