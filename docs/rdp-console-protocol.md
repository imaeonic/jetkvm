# RDP console local bridge

The RDP console prototype keeps `jetkvm_app` as the owner of native HDMI capture and USB gadget/HID state. A separate RDP daemon connects over `/run/jetkvm-rdp.sock`.

Each frame is `type:u8`, `length:u32-le`, then `payload`.

Protocol version: 1.

JetKVM to daemon: HELLO_ACK (0x02), VIDEO_STATE (0x05), VIDEO_FRAME (0x06), ERROR (0x07).

Daemon to JetKVM: HELLO (0x01), VIDEO_START (0x03), VIDEO_STOP (0x04), KEYBOARD_STATE (0x10), ABS_MOUSE (0x11), REL_MOUSE (0x12), WHEEL (0x13), DESKTOP_REQUEST (0x14).

`VIDEO_FRAME` contains the already hardware-encoded H.264 payload and must not be decoded/re-encoded by the transport. `DESKTOP_REQUEST` is deliberately advisory in the first prototype and is the future EDID/multi-monitor hook after capture-width limits are measured.
