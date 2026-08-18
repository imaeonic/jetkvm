package kvm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jetkvm/kvm/internal/native"
)

const (
	rdpBridgeProtocolVersion uint16 = 1
	rdpBridgeSocketDefault          = "/run/jetkvm-rdp.sock"
	rdpBridgeMaxMessage             = 16 * 1024 * 1024
	rdpBridgeVideoQueue             = 3
)

const (
	rdpMsgHello      byte = 0x01
	rdpMsgHelloAck   byte = 0x02
	rdpMsgVideoStart byte = 0x03
	rdpMsgVideoStop  byte = 0x04
	rdpMsgVideoState byte = 0x05
	rdpMsgVideoFrame byte = 0x06
	rdpMsgError      byte = 0x07

	rdpMsgKeyboardState  byte = 0x10
	rdpMsgAbsMouse       byte = 0x11
	rdpMsgRelMouse       byte = 0x12
	rdpMsgWheel          byte = 0x13
	rdpMsgDesktopRequest byte = 0x14
)

type rdpBridgeMessage struct {
	typ     byte
	payload []byte
}

type rdpBridgeClient struct {
	conn net.Conn

	send chan rdpBridgeMessage
	done chan struct{}
	once sync.Once

	videoStarted atomic.Bool
}

func (c *rdpBridgeClient) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

type rdpBridgeServer struct {
	mu     sync.Mutex
	client *rdpBridgeClient
}

var (
	rdpBridge           = &rdpBridgeServer{}
	rdpActiveSessions   atomic.Int32
	rdpVideoLifecycleMu sync.Mutex
)

func getRDPActiveSessions() int {
	return int(rdpActiveSessions.Load())
}

func getTotalActiveSessions() int {
	return getActiveSessions() + getRDPActiveSessions()
}

func rdpBridgeSocketPath() string {
	if path := os.Getenv("JETKVM_RDP_SOCKET"); path != "" {
		return path
	}
	return rdpBridgeSocketDefault
}

func initRDPBridge() {
	go func() {
		if err := rdpBridge.serve(appCtx, rdpBridgeSocketPath()); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error().Err(err).Msg("RDP bridge stopped")
		}
	}()
}

func (s *rdpBridgeServer) serve(ctx context.Context, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create RDP bridge socket directory: %w", err)
	}
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listen on RDP bridge socket %s: %w", path, err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(path)
	}()
	_ = os.Chmod(path, 0660)

	logger.Info().Str("path", path).Msg("RDP bridge listening")

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warn().Err(err).Msg("RDP bridge accept failed")
			continue
		}

		client := &rdpBridgeClient{
			conn: conn,
			send: make(chan rdpBridgeMessage, rdpBridgeVideoQueue+8),
			done: make(chan struct{}),
		}

		s.mu.Lock()
		old := s.client
		s.client = client
		s.mu.Unlock()
		if old != nil {
			old.close()
		}

		logger.Info().Msg("RDP bridge daemon connected")
		go s.runClient(client)
	}
}

func (s *rdpBridgeServer) runClient(client *rdpBridgeClient) {
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-client.done:
				return
			case msg := <-client.send:
				if err := writeRDPBridgeMessage(client.conn, msg.typ, msg.payload); err != nil {
					logger.Debug().Err(err).Msg("RDP bridge write failed")
					client.close()
					return
				}
			}
		}
	}()

	for {
		typ, payload, err := readRDPBridgeMessage(client.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				logger.Debug().Err(err).Msg("RDP bridge read failed")
			}
			break
		}
		if err := s.handleClientMessage(client, typ, payload); err != nil {
			logger.Warn().Err(err).Uint8("type", typ).Msg("invalid RDP bridge message")
			s.enqueue(client, rdpBridgeMessage{typ: rdpMsgError, payload: []byte(err.Error())})
		}
	}

	if client.videoStarted.Swap(false) {
		stopRDPVideoSession()
	}
	client.close()
	<-writerDone

	s.mu.Lock()
	if s.client == client {
		s.client = nil
	}
	s.mu.Unlock()
	logger.Info().Msg("RDP bridge daemon disconnected")
}

func (s *rdpBridgeServer) handleClientMessage(client *rdpBridgeClient, typ byte, payload []byte) error {
	switch typ {
	case rdpMsgHello:
		if len(payload) != 2 {
			return fmt.Errorf("HELLO payload length %d", len(payload))
		}
		version := binary.LittleEndian.Uint16(payload)
		if version != rdpBridgeProtocolVersion {
			return fmt.Errorf("unsupported bridge protocol %d", version)
		}
		ack := make([]byte, 2)
		binary.LittleEndian.PutUint16(ack, rdpBridgeProtocolVersion)
		s.enqueue(client, rdpBridgeMessage{typ: rdpMsgHelloAck, payload: ack})
		s.sendVideoStateTo(client, lastVideoState)
		return nil

	case rdpMsgVideoStart:
		if len(payload) != 1 {
			return fmt.Errorf("VIDEO_START payload length %d", len(payload))
		}
		if payload[0] != 0 {
			return fmt.Errorf("only H.264 is supported by the RDP bridge prototype")
		}
		if client.videoStarted.Load() {
			return nil
		}
		if err := startRDPVideoSession(); err != nil {
			return err
		}
		client.videoStarted.Store(true)
		return nil

	case rdpMsgVideoStop:
		if client.videoStarted.Swap(false) {
			stopRDPVideoSession()
		}
		return nil

	case rdpMsgKeyboardState:
		if len(payload) != 7 {
			return fmt.Errorf("KEYBOARD_STATE payload length %d", len(payload))
		}
		keys := append([]byte(nil), payload[1:]...)
		return rpcKeyboardReport(payload[0], keys)

	case rdpMsgAbsMouse:
		if len(payload) != 5 {
			return fmt.Errorf("ABS_MOUSE payload length %d", len(payload))
		}
		x := int(binary.LittleEndian.Uint16(payload[0:2]))
		y := int(binary.LittleEndian.Uint16(payload[2:4]))
		return rpcAbsMouseReport(x, y, payload[4])

	case rdpMsgRelMouse:
		if len(payload) != 3 {
			return fmt.Errorf("REL_MOUSE payload length %d", len(payload))
		}
		return rpcRelMouseReport(int8(payload[0]), int8(payload[1]), payload[2])

	case rdpMsgWheel:
		if len(payload) != 2 {
			return fmt.Errorf("WHEEL payload length %d", len(payload))
		}
		return rpcWheelReport(int8(payload[0]), int8(payload[1]))

	case rdpMsgDesktopRequest:
		if len(payload) != 4 {
			return fmt.Errorf("DESKTOP_REQUEST payload length %d", len(payload))
		}
		width := binary.LittleEndian.Uint16(payload[0:2])
		height := binary.LittleEndian.Uint16(payload[2:4])
		logger.Info().Uint16("width", width).Uint16("height", height).Msg("RDP client requested desktop geometry")
		// The request is deliberately advisory in the first prototype. Once the
		// capture-path limits are proven, this becomes the hook that applies a
		// generated EDID for /span or /multimon sessions.
		return nil
	}

	return fmt.Errorf("unknown RDP bridge message type 0x%02x", typ)
}

func startRDPVideoSession() error {
	rdpVideoLifecycleMu.Lock()
	defer rdpVideoLifecycleMu.Unlock()

	if rdpActiveSessions.Load() > 0 {
		return nil
	}
	if getActiveSessions() > 0 {
		return fmt.Errorf("a WebRTC console session is active; RDP and WebRTC video transports are mutually exclusive in this prototype")
	}

	rdpActiveSessions.Store(1)
	stopVideoSleepModeTicker()
	if err := setHostDisplayAdvertised(true, "rdp_session_connected", false); err != nil {
		rdpActiveSessions.Store(0)
		return fmt.Errorf("advertise host display: %w", err)
	}
	if err := nativeInstance.VideoSetCodecType(0); err != nil {
		rdpActiveSessions.Store(0)
		return fmt.Errorf("select H.264 for RDP: %w", err)
	}
	if err := nativeInstance.VideoStart(); err != nil {
		rdpActiveSessions.Store(0)
		return fmt.Errorf("start native video for RDP: %w", err)
	}

	onActiveSessionsChanged()
	if mqttManager != nil {
		mqttManager.publishSessionsState()
	}
	logger.Info().Msg("RDP video session started")
	return nil
}

func stopRDPVideoSession() {
	rdpVideoLifecycleMu.Lock()
	defer rdpVideoLifecycleMu.Unlock()

	if rdpActiveSessions.Swap(0) == 0 {
		return
	}

	_ = rpcKeyboardReport(0, keyboardClearStateKeys)
	if getActiveSessions() == 0 {
		_ = nativeInstance.VideoStop()
		_ = applyHostDisplayAdvertisement("rdp_session_disconnected")
		startVideoSleepModeTicker()
	}
	onActiveSessionsChanged()
	if mqttManager != nil {
		mqttManager.publishSessionsState()
	}
	logger.Info().Msg("RDP video session stopped")
}

func (s *rdpBridgeServer) publishVideoState(state native.VideoState) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client != nil {
		s.sendVideoStateTo(client, state)
	}
}

func (s *rdpBridgeServer) sendVideoStateTo(client *rdpBridgeClient, state native.VideoState) {
	payload := make([]byte, 13)
	if state.Ready {
		payload[0] = 1
	}
	binary.LittleEndian.PutUint16(payload[1:3], clampU16(state.Width))
	binary.LittleEndian.PutUint16(payload[3:5], clampU16(state.Height))
	fpsMilli := uint32(0)
	if state.FramePerSecond > 0 {
		fpsMilli = uint32(state.FramePerSecond * 1000)
	}
	binary.LittleEndian.PutUint32(payload[5:9], fpsMilli)
	binary.LittleEndian.PutUint32(payload[9:13], uint32(getTotalActiveSessions()))
	s.enqueue(client, rdpBridgeMessage{typ: rdpMsgVideoState, payload: payload})
}

func (s *rdpBridgeServer) publishVideoFrame(frame []byte, duration time.Duration, state native.VideoState) {
	if getRDPActiveSessions() == 0 {
		return
	}

	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil || !client.videoStarted.Load() {
		return
	}

	// Native may reuse its backing buffer after the callback returns. Keep the
	// bridge asynchronous by copying once here, then drop frames rather than
	// ever blocking the capture callback when the RDP client is behind.
	data := append([]byte(nil), frame...)
	payload := make([]byte, 9+len(data))
	binary.LittleEndian.PutUint32(payload[0:4], uint32(duration.Microseconds()))
	binary.LittleEndian.PutUint16(payload[4:6], clampU16(state.Width))
	binary.LittleEndian.PutUint16(payload[6:8], clampU16(state.Height))
	payload[8] = 0 // H.264 / AVC420
	copy(payload[9:], data)

	select {
	case client.send <- rdpBridgeMessage{typ: rdpMsgVideoFrame, payload: payload}:
	default:
		// Prefer latency over completeness. H.264 keyframes will recover a
		// dropped dependent chain and IronRDP adds its own client-side flow control.
	}
}

func (s *rdpBridgeServer) enqueue(client *rdpBridgeClient, msg rdpBridgeMessage) {
	select {
	case <-client.done:
		return
	case client.send <- msg:
	default:
		if msg.typ != rdpMsgVideoFrame {
			logger.Warn().Uint8("type", msg.typ).Msg("RDP bridge control queue full")
		}
	}
}

func clampU16(v int) uint16 {
	if v <= 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

func readRDPBridgeMessage(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	length := binary.LittleEndian.Uint32(header[1:5])
	if length > rdpBridgeMaxMessage {
		return 0, nil, fmt.Errorf("RDP bridge message too large: %d", length)
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

func writeRDPBridgeMessage(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > rdpBridgeMaxMessage {
		return fmt.Errorf("RDP bridge message too large: %d", len(payload))
	}
	header := [5]byte{typ, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
