package kvm

import "github.com/jetkvm/kvm/internal/usbgadget"

type UsbEndpointReport struct {
	ExceedsBudget bool `json:"exceedsBudget"`
}

func rpcGetUsbEndpointReport(devices usbgadget.Devices) (UsbEndpointReport, error) {
	return UsbEndpointReport{ExceedsBudget: usbgadget.ExceedsEndpointBudget(&devices)}, nil
}

func makeRDPUSBDevices(devices usbgadget.Devices) usbgadget.Devices {
	devices.Ncm = true
	if usbgadget.ExceedsEndpointBudget(&devices) && devices.RelativeMouse {
		devices.RelativeMouse = false
	}
	return devices
}

func rpcSetUsbDevicesRDP(devices usbgadget.Devices) error {
	return rpcSetUsbDevices(makeRDPUSBDevices(devices))
}

func rpcSetUsbDeviceStateRDP(device string, enabled bool) error {
	if device == "ncm" {
		// CDC-NCM is the transport for the RDP prototype and remains enabled.
		config.UsbDevices.Ncm = true
		gadget.SetGadgetDevices(effectiveUsbDevices())
		return updateUsbRelatedConfig()
	}
	return rpcSetUsbDeviceState(device, enabled)
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
