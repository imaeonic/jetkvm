package kvm

import (
	"testing"

	"github.com/jetkvm/kvm/internal/usbgadget"
)

func TestNormalizeRDPUSBDevicesReservesNCM(t *testing.T) {
	devices := usbgadget.Devices{
		AbsoluteMouse: true,
		RelativeMouse: true,
		Keyboard:      true,
		MassStorage:   true,
		SerialConsole: true,
		Audio:         true,
	}

	got := normalizeRDPUSBDevices(devices)
	if !got.Ncm {
		t.Fatal("CDC-NCM must remain enabled")
	}
	if !got.Keyboard || !got.AbsoluteMouse || !got.MassStorage {
		t.Fatal("KVM-critical keyboard, absolute mouse and mass storage must be preserved")
	}
	if usbgadget.ExceedsEndpointBudget(&got) {
		t.Fatal("normalized RDP USB profile exceeds endpoint budget")
	}
}
