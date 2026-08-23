package kvm

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"sync/atomic"
)

const h264ProbeMarkerFile = "/userdata/jetkvm/h264-probe.enable"

var h264ProbeFrameCount atomic.Uint32

func h264ProbeEnabled() bool {
	_, err := os.Stat(h264ProbeMarkerFile)
	return err == nil
}

func debugH264Frame(frame []byte) {
	if !h264ProbeEnabled() {
		return
	}

	n := h264ProbeFrameCount.Add(1)
	if n > 20 {
		return
	}

	format, nalTypes := analyseH264Frame(frame)
	headLen := len(frame)
	if headLen > 16 {
		headLen = 16
	}

	hasSPS := containsNALType(nalTypes, 7)
	hasPPS := containsNALType(nalTypes, 8)
	hasIDR := containsNALType(nalTypes, 5)

	nativeLogger.Warn().
		Uint32("probeFrame", n).
		Int("bytes", len(frame)).
		Str("format", format).
		Interface("nalTypes", nalTypes).
		Bool("sps", hasSPS).
		Bool("pps", hasPPS).
		Bool("idr", hasIDR).
		Str("head", hex.EncodeToString(frame[:headLen])).
		Msg("H264 probe frame")
}

func containsNALType(types []uint8, want uint8) bool {
	for _, typ := range types {
		if typ == want {
			return true
		}
	}
	return false
}

func analyseH264Frame(data []byte) (string, []uint8) {
	if types := annexBNALTypes(data); len(types) > 0 {
		return "annex-b", types
	}
	if types, ok := avccNALTypes(data); ok {
		return "avcc-4byte", types
	}
	if len(data) > 0 {
		typ := data[0] & 0x1f
		if typ >= 1 && typ <= 23 {
			return "raw-single-nal", []uint8{typ}
		}
	}
	return "unknown", nil
}

func annexBNALTypes(data []byte) []uint8 {
	var out []uint8
	for i := 0; i+3 <= len(data); {
		payload := -1
		if i+4 <= len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			payload = i + 4
			i += 4
		} else if data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
			payload = i + 3
			i += 3
		} else {
			i++
			continue
		}
		if payload < len(data) {
			out = append(out, data[payload]&0x1f)
		}
	}
	return out
}

func avccNALTypes(data []byte) ([]uint8, bool) {
	if len(data) < 5 {
		return nil, false
	}

	var out []uint8
	for offset := 0; offset < len(data); {
		if offset+4 > len(data) {
			return nil, false
		}
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if length <= 0 || offset+length > len(data) {
			return nil, false
		}
		typ := data[offset] & 0x1f
		if typ < 1 || typ > 23 {
			return nil, false
		}
		out = append(out, typ)
		offset += length
	}
	return out, len(out) > 0
}
