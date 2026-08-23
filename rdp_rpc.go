package kvm

import "github.com/jetkvm/kvm/internal/usbgadget"

type UsbEndpointReport struct {
	ExceedsBudget bool `json:"exceedsBudget"`
}

func rpcGetUsbEndpointReport(devices usbgadget.Devices) (UsbEndpointReport, error) {
	return UsbEndpointReport{ExceedsBudget: usbgadget.ExceedsEndpointBudget(&devices)}, nil
}

func rpcSetUsbDeviceStateRDP(device string, enabled bool) error {
	if device != "ncm" {
		return rpcSetUsbDeviceState(device, enabled)
	}
	config.UsbDevices.Ncm = enabled
	gadget.SetGadgetDevices(effectiveUsbDevices())
	return updateUsbRelatedConfig()
}

type RDPBridgeStatus struct {
	Enabled        bool   `json:"enabled"`
	ListenPort     int    `json:"listenPort"`
	Target         string `json:"target"`
	TargetReachable bool  `json:"targetReachable"`
	ActiveSessions int64  `json:"activeSessions"`
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

// Extend the existing RPC table without replacing jsonrpc.go. This keeps the
// R&D branch close to current upstream while adding the NCM endpoint-budget
// query expected by the imported USB settings UI and an RDP status endpoint.
func init() {
	rpcHandlers["getUsbEndpointReport"] = RPCHandler{Func: rpcGetUsbEndpointReport, Params: []string{"devices"}}
	rpcHandlers["setUsbDeviceState"] = RPCHandler{Func: rpcSetUsbDeviceStateRDP, Params: []string{"device", "enabled"}}
	rpcHandlers["getRdpBridgeStatus"] = RPCHandler{Func: rpcGetRDPBridgeStatus}
}
