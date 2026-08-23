package kvm

import "github.com/jetkvm/kvm/internal/usbgadget"

// prepareRDPUSBProfile enables CDC-NCM for this R&D branch while preserving
// the normal KVM keyboard, absolute mouse and virtual-media functions. The
// RV1106 has a tight IN-endpoint budget, so relative mouse is released when
// necessary; absolute mouse remains available for normal browser KVM control.
func prepareRDPUSBProfile() {
	if config == nil || config.UsbDevices == nil {
		return
	}

	devices := *config.UsbDevices
	devices.Ncm = true

	if usbgadget.ExceedsEndpointBudget(&devices) && devices.RelativeMouse {
		devices.RelativeMouse = false
		logger.Info().Msg("RDP prototype disabled relative mouse to free a USB endpoint for CDC-NCM; absolute mouse remains enabled")
	}

	if usbgadget.ExceedsEndpointBudget(&devices) {
		logger.Error().Msg("RDP USB profile still exceeds the RV1106 endpoint budget")
	}

	config.UsbDevices = &devices
}
