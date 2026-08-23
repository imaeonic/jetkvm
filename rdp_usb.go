package kvm

import "github.com/jetkvm/kvm/internal/usbgadget"

// normalizeRDPUSBDevices reserves enough RV1106 endpoints for CDC-NCM while
// preserving the KVM-critical keyboard, absolute mouse and virtual media.
func normalizeRDPUSBDevices(devices usbgadget.Devices) usbgadget.Devices {
	devices.Ncm = true

	if usbgadget.ExceedsEndpointBudget(&devices) && devices.RelativeMouse {
		devices.RelativeMouse = false
	}
	if usbgadget.ExceedsEndpointBudget(&devices) && devices.SerialConsole {
		devices.SerialConsole = false
	}
	if usbgadget.ExceedsEndpointBudget(&devices) && devices.Audio {
		// Native RDP audio does not depend on the JetKVM USB-audio gadget.
		devices.Audio = false
	}

	return devices
}

// prepareRDPUSBProfile enables the private USB network before gadget creation.
// Absolute mouse and keyboard remain available to the normal browser KVM.
func prepareRDPUSBProfile() {
	if config == nil || config.UsbDevices == nil {
		return
	}

	original := *config.UsbDevices
	devices := normalizeRDPUSBDevices(original)

	if original.RelativeMouse && !devices.RelativeMouse {
		logger.Info().Msg("RDP prototype disabled relative mouse to reserve a USB endpoint; absolute mouse remains enabled")
	}
	if original.SerialConsole && !devices.SerialConsole {
		logger.Info().Msg("RDP prototype disabled USB serial console to reserve endpoints for CDC-NCM")
	}
	if original.Audio && !devices.Audio {
		logger.Info().Msg("RDP prototype disabled USB audio gadget to reserve endpoints; native RDP audio is unaffected")
	}
	if usbgadget.ExceedsEndpointBudget(&devices) {
		logger.Error().Msg("RDP USB profile exceeds the RV1106 endpoint budget even after normalization")
	}

	config.UsbDevices = &devices
}
