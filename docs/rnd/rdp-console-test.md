# RDP over USB - prototype validation

Branch: `rnd/rdp-console`

## What this prototype does

The JetKVM remains a composite USB device and presents the target with normal JetKVM HID/storage functions plus a CDC-NCM USB Ethernet adapter.

The private USB link is:

- JetKVM: `172.16.55.1/24`
- target: `172.16.55.2/24`, assigned automatically by the JetKVM DHCP service
- no default gateway or DNS is advertised, so this interface does not replace the target's normal network route

JetKVM exposes TCP and UDP port 3389 on its normal LAN address and transparently relays them to `172.16.55.2:3389`.

Because the connection still terminates in the Windows Remote Desktop service, Windows and mstsc retain native RDP features including:

- multi-monitor / span displays;
- clipboard redirection;
- local drive redirection;
- remote audio;
- NLA/CredSSP/TLS;
- RDP UDP transport where available.

The existing browser KVM remains available and continues to use the physical HDMI capture and USB HID keyboard/mouse for BIOS, POST, recovery and other out-of-band access.

## Prerequisites

- Original JetKVM with a system image containing USB Ethernet/configfs and nftables kernel support.
- JetKVM USB data connection attached to the target PC.
- Windows edition capable of hosting Remote Desktop, such as Pro, Enterprise or Server.
- Remote Desktop enabled on Windows.

No JetKVM-specific software or driver is installed on Windows. CDC-NCM uses the Windows inbox USB networking driver.

## Deploy

Build/deploy the application from this branch using the normal JetKVM development workflow:

```sh
./dev_deploy.sh -r <JETKVM_IP>
```

The GitHub `build` workflow also publishes the `jetkvm-rdp-prototype` artifact containing `bin/jetkvm_app`.

## Expected USB state

After the application starts, Windows should enumerate a new USB Ethernet adapter and receive:

```text
IPv4 address: 172.16.55.2
Subnet mask:  255.255.255.0
Gateway:      none
DNS:          none
```

JetKVM retains keyboard and absolute-mouse HID control. The prototype may disable the relative-mouse function to stay within the RV1106 USB endpoint budget.

## Windows firewall

Windows can classify the new USB network as Public. Some Windows installations enable the built-in Remote Desktop firewall rules only for Domain/Private profiles.

If the JetKVM UI reports `Waiting for Windows RDP` even though Remote Desktop is enabled, enable the existing built-in Remote Desktop rules for the active profile from an elevated PowerShell prompt:

```powershell
Get-NetFirewallRule -DisplayGroup "Remote Desktop" | Set-NetFirewallRule -Enabled True
```

This is a Windows configuration change only; it installs no agent, driver or third-party software.

## Test from a Windows client

Open Remote Desktop Connection and connect to the JetKVM's normal LAN IP or hostname, not `172.16.55.2`.

For all monitors:

```cmd
mstsc /multimon
```

Before connecting, under Local Resources / More, enable the resources to validate:

- Clipboard
- Drives
- Remote audio playback

Authenticate with the target Windows credentials. The Windows RDP certificate/NLA exchange passes through JetKVM unchanged.

## Acceptance tests

1. JetKVM browser KVM still displays HDMI and accepts keyboard/mouse input.
2. Windows enumerates CDC-NCM without a custom driver.
3. Windows receives `172.16.55.2` automatically.
4. JetKVM UI changes from `Waiting for Windows RDP` to `Windows RDP ready`.
5. `mstsc <JETKVM_IP>` reaches the target Windows login.
6. `mstsc /multimon` uses all selected client monitors.
7. Clipboard works in both directions through native RDP.
8. Redirected client drives appear in the Windows RDP session.
9. Windows audio is heard on the RDP client.
10. RDP reconnect works after closing/reopening mstsc.
11. Browser KVM remains usable after RDP disconnect.
12. Rebooting the target leaves physical browser KVM available through POST/BIOS while native RDP is unavailable.
13. Once Windows and TermService return, the RDP status becomes ready again without reconfiguring the JetKVM.

## Observability

The prototype exports:

```text
jetkvm_rdp_bridge_active_sessions
jetkvm_rdp_bridge_target_up
jetkvm_rdp_bridge_connections_total
jetkvm_rdp_bridge_target_dial_failures_total
```

The USB settings page also shows target reachability and current active RDP sessions.

## Current architectural boundary

This prototype intentionally does not attempt to make mstsc display the physical HDMI/BIOS console when Windows RDP is unavailable. Native RDP mode and the browser hardware-KVM mode coexist, but BIOS/POST remains in the browser KVM. A future hardware-RDP fallback would require an RDP server implementation capable of presenting the physical capture stream itself.
