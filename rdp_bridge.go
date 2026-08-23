package kvm

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jetkvm/kvm/internal/usbgadget"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	rdpBridgeListenPort = 3389
	rdpTargetPort       = 3389
)

var (
	rdpBridgeActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "jetkvm_rdp_bridge_active_sessions",
		Help: "Current number of RDP TCP sessions proxied to the USB-attached target",
	})
	rdpBridgeTargetUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "jetkvm_rdp_bridge_target_up",
		Help: "Whether the target Windows RDP service is reachable over USB-NCM",
	})
	rdpBridgeConnections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "jetkvm_rdp_bridge_connections_total",
		Help: "RDP TCP connections accepted by the JetKVM bridge",
	})
	rdpBridgeDialFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "jetkvm_rdp_bridge_target_dial_failures_total",
		Help: "RDP bridge attempts that could not reach the target RDP service",
	})
	rdpBridgeSessionCount    atomic.Int64
	rdpBridgeTargetReachable atomic.Bool
)

func rdpTargetAddress() string {
	return net.JoinHostPort(usbgadget.NCMPeerIPv4, strconv.Itoa(rdpTargetPort))
}

func rdpDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: net.ParseIP("172.16.55.1")},
	}
}

// initRDPBridge exposes the target's native Windows RDP service on JetKVM's
// TCP/3389. The stream is intentionally opaque: TLS, CredSSP/NLA, graphics,
// multi-monitor, clipboard, redirected drives and audio remain end-to-end
// between mstsc and Windows instead of being reimplemented by JetKVM.
func initRDPBridge(ctx context.Context) {
	go runRDPBridge(ctx)
	go runRDPUDPBridge(ctx)
	go monitorRDPTarget(ctx)
}

func runRDPBridge(ctx context.Context) {
	listener, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(rdpBridgeListenPort)))
	if err != nil {
		logger.Error().Err(err).Int("port", rdpBridgeListenPort).Msg("RDP bridge failed to listen")
		return
	}
	defer listener.Close()

	logger.Info().Int("port", rdpBridgeListenPort).Str("target", rdpTargetAddress()).Msg("RDP bridge listening")

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			logger.Warn().Err(err).Msg("RDP bridge accept failed")
			continue
		}
		rdpBridgeConnections.Inc()
		go proxyRDPSession(client)
	}
}

func proxyRDPSession(client net.Conn) {
	defer client.Close()

	target, err := rdpDialer(4*time.Second).Dial("tcp", rdpTargetAddress())
	if err != nil {
		rdpBridgeDialFailures.Inc()
		rdpBridgeTargetUp.Set(0)
		rdpBridgeTargetReachable.Store(false)
		logger.Debug().Err(err).Str("target", rdpTargetAddress()).Msg("RDP target unavailable")
		return
	}
	defer target.Close()

	rdpBridgeTargetUp.Set(1)
	rdpBridgeTargetReachable.Store(true)
	count := rdpBridgeSessionCount.Add(1)
	rdpBridgeActive.Set(float64(count))
	defer func() {
		count := rdpBridgeSessionCount.Add(-1)
		rdpBridgeActive.Set(float64(count))
	}()

	if c, ok := client.(*net.TCPConn); ok {
		_ = c.SetKeepAlive(true)
		_ = c.SetKeepAlivePeriod(30 * time.Second)
		_ = c.SetNoDelay(true)
	}
	if c, ok := target.(*net.TCPConn); ok {
		_ = c.SetKeepAlive(true)
		_ = c.SetKeepAlivePeriod(30 * time.Second)
		_ = c.SetNoDelay(true)
	}

	done := make(chan struct{}, 2)
	copyHalf := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyHalf(target, client)
	go copyHalf(client, target)
	<-done
}

func monitorRDPTarget(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		probeRDPTarget()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func probeRDPTarget() {
	conn, err := rdpDialer(1200*time.Millisecond).Dial("tcp", rdpTargetAddress())
	if err != nil {
		rdpBridgeTargetUp.Set(0)
		rdpBridgeTargetReachable.Store(false)
		return
	}
	rdpBridgeTargetUp.Set(1)
	rdpBridgeTargetReachable.Store(true)
	_ = conn.Close()
}
