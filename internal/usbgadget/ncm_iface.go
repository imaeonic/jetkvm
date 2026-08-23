package usbgadget

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/vishvananda/netlink"
)

const ncmInterfaceName = "usb0"

// bringUpNcmInterface brings up usb0, assigns JetKVM a deterministic private
// IPv4 address, installs host isolation, and starts the single-host DHCP
// service. The short retry handles configfs creating usb0 asynchronously after
// UDC bind.
func (u *UsbGadget) bringUpNcmInterface() error {
	var link netlink.Link
	var err error
	for i := 0; i < 20; i++ {
		link, err = netlink.LinkByName(ncmInterfaceName)
		if err == nil {
			break
		}
		var lnf netlink.LinkNotFoundError
		if !errors.As(err, &lnf) {
			return fmt.Errorf("lookup %s: %w", ncmInterfaceName, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("%s did not appear after gadget bind: %w", ncmInterfaceName, err)
	}

	_ = os.WriteFile("/proc/sys/net/ipv6/conf/"+ncmInterfaceName+"/disable_ipv6", []byte("0"), 0644)

	addr, err := netlink.ParseAddr(NCMServerIPv4CIDR)
	if err != nil {
		return fmt.Errorf("parse NCM address: %w", err)
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("set %s address: %w", ncmInterfaceName, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("link up %s: %w", ncmInterfaceName, err)
	}

	// Install isolation before opening DHCP. Fail closed if either stage fails.
	if err := u.applyNcmFirewall(); err != nil {
		_ = netlink.LinkSetDown(link)
		return fmt.Errorf("apply NCM firewall: %w", err)
	}
	if err := u.startNcmDHCP(); err != nil {
		u.removeNcmFirewall()
		_ = netlink.LinkSetDown(link)
		return fmt.Errorf("start NCM DHCP: %w", err)
	}

	u.log.Info().Str("address", NCMServerIPv4CIDR).Str("peer", NCMPeerIPv4).Msg("USB Ethernet ready")
	return nil
}

func (u *UsbGadget) tearDownNcmInterface() {
	u.stopNcmDHCP()
	u.removeNcmFirewall()
	link, err := netlink.LinkByName(ncmInterfaceName)
	if err != nil {
		return
	}
	_ = netlink.LinkSetDown(link)
}
