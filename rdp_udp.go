package kvm

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/jetkvm/kvm/internal/usbgadget"
)

type rdpUDPRelay struct {
	conn   *net.UDPConn
	client *net.UDPAddr
}

var (
	rdpUDPRelaysMu sync.Mutex
	rdpUDPRelays   = make(map[string]*rdpUDPRelay)
)

func runRDPUDPBridge(ctx context.Context) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{Port: rdpBridgeListenPort})
	if err != nil {
		logger.Warn().Err(err).Int("port", rdpBridgeListenPort).Msg("RDP UDP bridge failed to listen; TCP RDP remains available")
		return
	}
	defer listener.Close()

	logger.Info().Int("port", rdpBridgeListenPort).Msg("RDP UDP bridge listening")
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	buf := make([]byte, 64*1024)
	for {
		n, client, err := listener.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}

		relay, err := getRDPUDPRelay(listener, client)
		if err != nil {
			logger.Debug().Err(err).Msg("failed to create RDP UDP target relay")
			continue
		}
		_ = relay.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := relay.conn.Write(buf[:n]); err != nil {
			removeRDPUDPRelay(client.String(), relay)
		}
	}
}

func getRDPUDPRelay(listener *net.UDPConn, client *net.UDPAddr) (*rdpUDPRelay, error) {
	key := client.String()
	rdpUDPRelaysMu.Lock()
	if relay := rdpUDPRelays[key]; relay != nil {
		rdpUDPRelaysMu.Unlock()
		return relay, nil
	}
	rdpUDPRelaysMu.Unlock()

	target := &net.UDPAddr{IP: net.ParseIP(usbgadget.NCMPeerIPv4), Port: rdpTargetPort}
	local := &net.UDPAddr{IP: net.ParseIP("172.16.55.1")}
	conn, err := net.DialUDP("udp4", local, target)
	if err != nil {
		return nil, err
	}

	relay := &rdpUDPRelay{conn: conn, client: client}
	rdpUDPRelaysMu.Lock()
	if existing := rdpUDPRelays[key]; existing != nil {
		rdpUDPRelaysMu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	rdpUDPRelays[key] = relay
	rdpUDPRelaysMu.Unlock()

	go pumpRDPUDPReplies(listener, key, relay)
	return relay, nil
}

func pumpRDPUDPReplies(listener *net.UDPConn, key string, relay *rdpUDPRelay) {
	defer removeRDPUDPRelay(key, relay)
	buf := make([]byte, 64*1024)
	for {
		_ = relay.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		n, err := relay.conn.Read(buf)
		if err != nil {
			return
		}
		_ = listener.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := listener.WriteToUDP(buf[:n], relay.client); err != nil {
			return
		}
	}
}

func removeRDPUDPRelay(key string, relay *rdpUDPRelay) {
	rdpUDPRelaysMu.Lock()
	if rdpUDPRelays[key] == relay {
		delete(rdpUDPRelays, key)
	}
	rdpUDPRelaysMu.Unlock()
	_ = relay.conn.Close()
}
