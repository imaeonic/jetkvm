# JetKVM RDP Console Proof of Concept

## Goal

Present the physical console attached to JetKVM as a standard RDP endpoint, without requiring RDP support on the target machine.

The target remains controlled through JetKVM's existing hardware paths:

- HDMI capture for video
- USB HID for keyboard and mouse
- Existing ATX / virtual-media capabilities where available

The Windows RDP client connects to JetKVM itself.

## Target architecture

```text
mstsc / FreeRDP
       |
       | RDP
       v
JetKVM RDP service
       |
       +-- video <- JetKVM HDMI capture / encoder
       +-- keyboard -> JetKVM USB HID
       +-- mouse -> JetKVM USB HID
       +-- session geometry -> EDID policy
```

## Design principles

1. Keep the target OS-independent. The attached device may be Windows, Linux, a hypervisor, an NVR/DVR, firmware setup, recovery media or a machine with no operating system.
2. Reuse JetKVM's existing capture and HID paths rather than recreating them.
3. Avoid unnecessary video transcode stages. The preferred path is to reuse the hardware-produced H.264 stream if it can be transported through the negotiated RDP graphics mode.
4. Keep a recovery / management path while the RDP implementation is experimental. Do not remove SSH or the existing application until the RDP path is proven.
5. Treat multi-monitor as a later capability built on top of a working single-display RDP console.

## Proof-of-concept gates

### Gate 1 - RDP transport

- Cross-compile a minimal RDP server for the JetKVM ARM userspace.
- Boot it on the device and listen on TCP/3389.
- Connect using Windows mstsc.
- Present a synthetic test framebuffer.

Success means a stock RDP client can establish and maintain a session to JetKVM.

### Gate 2 - input bridge

- Capture RDP keyboard events.
- Translate them into the existing JetKVM keyboard HID path.
- Capture RDP pointer events.
- Translate them into the existing JetKVM mouse HID path.
- Verify modifiers, special keys and absolute pointer coordinates.

Success means mstsc can control an attached target using JetKVM's physical USB connection.

### Gate 3 - real video

- Feed the JetKVM HDMI capture into the RDP session.
- Measure latency, CPU, memory and bitrate.
- Determine whether the existing hardware H.264 elementary stream can be carried into the negotiated RDP graphics path without decode/re-encode.
- If direct transport is not possible, benchmark the cheapest viable conversion path before accepting it.

Success means a usable live physical console is visible in mstsc.

### Gate 4 - console features

- Power / reset controls.
- Virtual media controls.
- Clipboard / text injection where practical.
- Connection status and safe disconnect behaviour.

### Gate 5 - multi-monitor / span

- Read the monitor layout requested by the RDP client.
- Derive an appropriate remote desktop geometry.
- Generate / select an EDID policy for the attached target.
- Validate HDMI receiver, capture pipeline and encoder maximum dimensions and pixel rates.
- Support the largest useful geometry the hardware can actually capture.

Example target:

```text
3 x 1920x1080 local displays
        ->
5760x1080 requested console surface
        ->
matching JetKVM EDID if the video pipeline supports it
```

## Important unresolved questions

- Maximum HDMI input width, height and pixel clock accepted by the JetKVM receiver path.
- Maximum frame width / height accepted by the RV1106 capture and encoder pipeline.
- Whether hardware H.264 output can be passed into the RDP graphics pipeline without transcoding.
- Memory footprint of a large logical desktop.
- Behaviour of BIOS / UEFI and appliances when presented with unusual ultrawide EDIDs.
- RDP security / authentication model suitable for an out-of-band management device.

## Hardware testing requirements

Once a physical JetKVM is available:

- JetKVM with Developer Mode enabled and SSH key configured.
- A Windows workstation with mstsc for client testing.
- A disposable or non-critical HDMI + USB target for early HID/video tests.
- Network access to the JetKVM development interface.
- Recovery capability retained before flashing experimental system images.

For low-level failures, use the model-appropriate JetKVM recovery mechanism. A serial console is highly desirable while changing boot/rootfs behaviour.

## Initial implementation policy

The first implementation will not remove the current web application. RDP will run alongside it until video, input, recovery and authentication are proven. Removing the browser UI can be considered only after the RDP console is demonstrably self-sufficient.
