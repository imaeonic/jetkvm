# RDP Console R&D

Branch: `rnd/rdp-console`

## Goal

Add an RDP-facing access path to JetKVM without losing normal hardware KVM behaviour.

The design has two modes:

1. **Enhanced Windows mode** - `mstsc` connects to JetKVM, which forwards the RDP transport over a private USB network link to the target Windows machine. Windows remains the RDP server, so native RDP features such as multi-monitor, clipboard, redirected drives and audio remain available.
2. **Hardware fallback mode** - when the target RDP service is unavailable, JetKVM terminates the RDP session itself and exposes the physical HDMI capture plus USB HID, preserving BIOS/POST/recovery access.

No custom target-side JetKVM software should be required for enhanced mode. The target only needs its normal Windows RDP service enabled.

## USB architecture

JetKVM remains a composite USB gadget. Adding Ethernet-over-USB must not replace keyboard or mouse functions.

Target-visible functions can include:

- keyboard HID
- absolute/relative mouse HID
- mass storage
- audio where enabled
- CDC-NCM Ethernet

Upstream PR `jetkvm/kvm#1470` already implements CDC-NCM and is the preferred starting point rather than creating a separate USB Ethernet implementation.

## Initial proof of concept

The quickest useful milestone is enhanced mode only:

```text
mstsc
  -> JetKVM eth0:3389
  -> TCP proxy
  -> private USB-NCM network
  -> Windows target:3389
```

If Windows terminates the RDP session, normal RDP capabilities are preserved automatically, including multi-monitor, clipboard, drive redirection and sound.

For deterministic target addressing, extend the NCM foundation with a small private IPv4 network and DHCP on `usb0` rather than relying on unpredictable IPv6/APIPA addressing.

Proposed private link:

```text
JetKVM usb0   172.16.55.1/24
Target DHCP   172.16.55.2/24
```

The USB network must remain isolated from the JetKVM management plane.

## Work order

1. Import/rebase the useful parts of upstream PR #1470 onto current `dev`.
2. Verify HID + NCM coexist within the RV1106 endpoint budget.
3. Add deterministic IPv4/DHCP on `usb0`.
4. Add an opt-in TCP/3389 proxy from JetKVM to the target over `usb0`.
5. Prove native Windows RDP features through the proxy: multi-monitor, clipboard, redirected drives and audio.
6. Add target-RDP availability detection and state reporting.
7. Add JetKVM-hosted RDP hardware fallback using HDMI capture + HID.
8. Add automatic enhanced/fallback mode selection without exposing the management plane to the target host.

## Non-goals for the first milestone

- Do not rewrite RDP protocol handling for enhanced mode.
- Do not install a custom driver or agent on Windows.
- Do not remove or replace existing JetKVM HID/mass-storage functions.
- Do not expose JetKVM management services over the USB network.
