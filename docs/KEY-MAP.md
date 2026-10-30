# CEC key map

Supported incoming User Control Pressed frames use opcode 0x44. The operand is
mapped to Linux evdev; each supported incoming press produces one press/release
pair. Held state is not preserved. Unsupported operands are logged and counted.

- Select 0x00 → KEY_ENTER; Up/Down/Left/Right 0x01–0x04 → direction keys.
- Root/Setup/Contents Menu 0x09–0x0b → KEY_MENU; Exit 0x0d → KEY_ESC.
- Play 0x44 → KEY_PLAY; Stop 0x45 → KEY_STOPCD; Pause 0x46 → KEY_PAUSECD.
- Record/Rewind/Fast Forward 0x47–0x49 → corresponding media keys.
- Next/Previous 0x4b/0x4c → KEY_NEXTSONG/KEY_PREVIOUSSONG.
- Power 0x40 → KEY_POWER; Volume Up/Down 0x41/0x42 → volume keys; Mute 0x43 → KEY_MUTE.

CEC Play 0x44 and Pause 0x46 are distinct commands, not a universal toggle.
The keyboard Pause key (119) and application Stop key (128) are not the media
Pause (201) and Stop (166) keys. Color buttons and multi-operand play functions
are not currently mapped. TV/compositor/application behavior requires testing.
