package usbgadget

import (
	"encoding/binary"
	"net"
	"testing"
)

func makeDHCPRequest(msgType byte) []byte {
	pkt := make([]byte, 240, 256)
	pkt[0] = 1
	pkt[1] = 1
	pkt[2] = 6
	binary.BigEndian.PutUint32(pkt[4:8], 0x12345678)
	copy(pkt[28:34], []byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55})
	binary.BigEndian.PutUint32(pkt[236:240], 0x63825363)
	pkt = append(pkt, 53, 1, msgType, 255)
	return pkt
}

func TestDHCPMessageType(t *testing.T) {
	if got := dhcpMessageType(makeDHCPRequest(1)); got != 1 {
		t.Fatalf("message type = %d, want 1", got)
	}
}

func TestBuildDHCPReply(t *testing.T) {
	req := makeDHCPRequest(1)
	reply, err := buildDHCPReply(req, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reply[0] != 2 {
		t.Fatalf("op = %d, want BOOTREPLY", reply[0])
	}
	if got := net.IP(reply[16:20]).String(); got != NCMPeerIPv4 {
		t.Fatalf("lease = %s, want %s", got, NCMPeerIPv4)
	}
	if got := net.IP(reply[20:24]).String(); got != "172.16.55.1" {
		t.Fatalf("server = %s, want 172.16.55.1", got)
	}
	if binary.BigEndian.Uint32(reply[4:8]) != 0x12345678 {
		t.Fatal("transaction ID was not preserved")
	}
	if got := dhcpMessageType(reply); got != 2 {
		t.Fatalf("reply DHCP type = %d, want OFFER", got)
	}
}
