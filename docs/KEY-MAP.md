# CEC Key Map

The final map will translate libCEC user-control codes to Linux evdev keys.
These are the initial interoperability targets:

| Remote action | Linux event |
|---|---|
| Up / Down / Left / Right | `KEY_UP` / `KEY_DOWN` / `KEY_LEFT` / `KEY_RIGHT` |
| Select / OK | `KEY_ENTER` |
| Back / Return | `KEY_ESC` or `KEY_BACK` |
| Play / Pause | `KEY_PLAYPAUSE` |
| Stop | `KEY_STOP` |
| Volume Up / Down | `KEY_VOLUMEUP` / `KEY_VOLUMEDOWN` |
| Mute | `KEY_MUTE` |
| Home / Menu | `KEY_HOME` / `KEY_MENU` |

CEC key presses use the standard User Control Pressed opcode `0x44`. The Play
operand is `0x44` and the Pause operand is `0x46`; they are not a universal
toggle. The daemon should emit the Linux media key directly rather than asking
applications to understand raw CEC frames.
