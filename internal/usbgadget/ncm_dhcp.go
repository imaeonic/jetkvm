package usbgadget

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	NCMServerIPv4CIDR = "172.16.55.1/24"
	NCMPeerIPv4       = "172.16.55.2"

	dhcpServerPort = 67
	dhcpClientPort = 68
)

var (
	ncmDHCPConn net.PacketConn
	ncmDHCPLock = make(chan struct{}, 1)
)

func lockNcmDHCP()   { ncmDHCPLock <- struct{}{} }
func unlockNcmDHCP() { <-ncmDHCPLock }

// startNcmDHCP starts a deliberately tiny, single-lease DHCPv4 server for the
// one host physically attached to JetKVM's USB gadget. It provides only an
// on-link address and subnet mask: no router or DNS option is advertised, so
// Windows will not replace its normal default route with the KVM USB link.
func (u *UsbGadget) startNcmDHCP() error {
	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var controlErr error
			if err := c.Control(func(fd uintptr) {
				if err := unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, ncmInterfaceName); err != nil {
					controlErr = err
					return
				}
				controlErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}

	conn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", dhcpServerPort))
	if err != nil {
		return fmt.Errorf("listen DHCP on %s: %w", ncmInterfaceName, err)
	}

	lockNcmDHCP()
	old := ncmDHCPConn
	ncmDHCPConn = conn
	unlockNcmDHCP()
	if old != nil {
		_ = old.Close()
	}

	go u.serveNcmDHCP(conn)
	return nil
}

func (u *UsbGadget) stopNcmDHCP() {
	lockNcmDHCP()
	conn := ncmDHCPConn
	ncmDHCPConn = nil
	unlockNcmDHCP()
	if conn != nil {
		_ = conn.Close()
	}
}

func (u *UsbGadget) serveNcmDHCP(conn net.PacketConn) {
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				u.log.Warn().Err(err).Msg("USB DHCP read failed")
			}
			return
		}

		req := append([]byte(nil), buf[:n]...)
		msgType := dhcpMessageType(req)
		var replyType byte
		switch msgType {
		case 1: // DHCPDISCOVER
			replyType = 2 // DHCPOFFER
		case 3: // DHCPREQUEST
			replyType = 5 // DHCPACK
		default:
			continue
		}

		reply, err := buildDHCPReply(req, replyType)
		if err != nil {
			u.log.Debug().Err(err).Msg("ignoring malformed USB DHCP packet")
			continue
		}
		if _, err := conn.WriteTo(reply, &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpClientPort}); err != nil && !errors.Is(err, net.ErrClosed) {
			u.log.Warn().Err(err).Msg("USB DHCP reply failed")
		}
	}
}

func dhcpMessageType(pkt []byte) byte {
	if len(pkt) < 240 || binary.BigEndian.Uint32(pkt[236:240]) != 0x63825363 {
		return 0
	}
	for i := 240; i < len(pkt); {
		code := pkt[i]
		i++
		switch code {
		case 0:
			continue
		case 255:
			return 0
		}
		if i >= len(pkt) {
			return 0
		}
		l := int(pkt[i])
		i++
		if i+l > len(pkt) {
			return 0
		}
		if code == 53 && l == 1 {
			return pkt[i]
		}
		i += l
	}
	return 0
}

func buildDHCPReply(req []byte, msgType byte) ([]byte, error) {
	if len(req) < 240 || req[0] != 1 || binary.BigEndian.Uint32(req[236:240]) != 0x63825363 {
		return nil, fmt.Errorf("invalid BOOTP/DHCP request")
	}

	reply := make([]byte, 240, 320)
	reply[0] = 2 // BOOTREPLY
	reply[1] = req[1]
	reply[2] = req[2]
	copy(reply[4:8], req[4:8])   // xid
	copy(reply[8:12], req[8:12]) // secs + flags

	serverIP := net.ParseIP("172.16.55.1").To4()
	peerIP := net.ParseIP(NCMPeerIPv4).To4()
	if serverIP == nil || peerIP == nil {
		return nil, fmt.Errorf("invalid NCM IPv4 constants")
	}
	copy(reply[16:20], peerIP)   // yiaddr
	copy(reply[20:24], serverIP) // siaddr
	copy(reply[28:44], req[28:44])
	binary.BigEndian.PutUint32(reply[236:240], 0x63825363)

	addOpt := func(code byte, data []byte) {
		reply = append(reply, code, byte(len(data)))
		reply = append(reply, data...)
	}
	addOpt(53, []byte{msgType})
	addOpt(54, serverIP)
	addOpt(1, net.IPv4(255, 255, 255, 0).To4())

	lease := make([]byte, 4)
	binary.BigEndian.PutUint32(lease, 24*60*60)
	addOpt(51, lease)

	t1 := make([]byte, 4)
	binary.BigEndian.PutUint32(t1, 12*60*60)
	addOpt(58, t1)

	t2 := make([]byte, 4)
	binary.BigEndian.PutUint32(t2, 21*60*60)
	addOpt(59, t2)

	reply = append(reply, 255)
	return reply, nil
}
