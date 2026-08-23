package kvm

import (
	"fmt"

	"github.com/jetkvm/kvm/internal/usbgadget"
)

type UsbEndpointReport struct {
	ExceedsBudget bool `json:"exceedsBudget"`
}

func rpcGetUsbEndpointReport(devices usbgadget.Devices) (UsbEndpointReport, error) {
	return UsbEndpointReport{ExceedsBudget: usbgadget.ExceedsEndpointBudget(&devices)}, nil
}

func rpcSetUsbDevicesRDP(devices usbgadget.Devices) error {
	return rpcSetUsbDevices(normalizeRDPUSBDevices(devices))
}

func rpcSetUsbDeviceStateRDP(device string, enabled bool) error {
	devices := *config.UsbDevices
	switch device {
	case "absoluteMouse":
		devices.AbsoluteMouse = enabled
	case "relativeMouse":
		devices.RelativeMouse = enabled
	case "keyboard":
		devices.Keyboard = enabled
	case "massStorage":
		devices.MassStorage = enabled
	case "serialConsole":
		devices.SerialConsole = enabled
	case "audio":
		devices.Audio = enabled
		if !enabled {
			config.AudioEnabled = false
		}
	case "ncm":
		// CDC-NCM is the required transport for this prototype.
		devices.Ncm = true
	default:
		return fmt.Errorf("invalid device: %s", device)
	}

	devices = normalizeRDPUSBDevices(devices)
	config.UsbDevices = &devices
	if !effectiveAudioEnabled() {
		stopAudio()
	}
	gadget.SetGadgetDevices(effectiveUsbDevices())
	return updateUsbRelatedConfig()
}

type RDPBridgeStatus struct {
	Enabled         bool   `json:"enabled"`
	ListenPort      int    `json:"listenPort"`
	Target          string `json:"target"`
	TargetReachable bool   `json:"targetReachable"`
	ActiveSessions  int64  `json:"activeSessions"`
}

func rpcGetRDPBridgeStatus() (RDPBridgeStatus, error) {
	return RDPBridgeStatus{
		Enabled:         true,
		ListenPort:      rdpBridgeListenPort,
		Target:          rdpTargetAddress(),
		TargetReachable: rdpBridgeTargetReachable.Load(),
		ActiveSessions:  rdpBridgeSessionCount.Load(),
	}, nil
}

// registerRDPRPCHandlers extends the existing RPC table without replacing
// jsonrpc.go, keeping the R&D branch close to current upstream.
func registerRDPRPCHandlers() {
	rpcHandlers["getUsbEndpointReport"] = RPCHandler{Func: rpcGetUsbEndpointReport, Params: []string{"devices"}}
	rpcHandlers["setUsbDevices"] = RPCHandler{Func: rpcSetUsbDevicesRDP, Params: []string{"devices"}}
	rpcHandlers["setUsbDeviceState"] = RPCHandler{Func: rpcSetUsbDeviceStateRDP, Params: []string{"device", "enabled"}}
	rpcHandlers["getRdpBridgeStatus"] = RPCHandler{Func: rpcGetRDPBridgeStatus}
}
