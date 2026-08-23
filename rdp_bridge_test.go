package kvm

import (
	"bytes"
	"testing"
)

func TestRDPBridgeMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	wantType := byte(0x42)
	wantPayload := []byte{1, 2, 3, 4, 5}
	if err := writeRDPBridgeMessage(&buf, wantType, wantPayload); err != nil {
		t.Fatal(err)
	}
	gotType, gotPayload, err := readRDPBridgeMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if gotType != wantType {
		t.Fatalf("type=%x want=%x", gotType, wantType)
	}
	if !bytes.Equal(gotPayload, wantPayload) {
		t.Fatalf("payload=%v want=%v", gotPayload, wantPayload)
	}
}

func TestRDPBridgeRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(1)
	buf.Write([]byte{1, 0, 0, 1}) // 16 MiB + 1, little endian
	if _, _, err := readRDPBridgeMessage(&buf); err == nil {
		t.Fatal("expected oversized message error")
	}
}
