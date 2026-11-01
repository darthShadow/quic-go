package congestion

import (
	"math"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
)

// quichePacer follows quiche 7980e3d5d89a with the measured 4 ms debt cap.
// The default sender retains the upstream token bucket in pacer.go.
type quichePacer struct {
	idealNextSend   monotime.Time
	burstTokens     uint64
	lumpyTokens     uint64
	maxLumpPackets  uint64
	pacingLimited   bool
	maxDatagramSize protocol.ByteCount
}

func newQuichePacer() *quichePacer {
	return &quichePacer{
		maxDatagramSize: protocol.InitialPacketSize,
		// quiche 7980e3d5d89a: pacing_sender.cc:15-27, initial burst.
		burstTokens:    10,
		maxLumpPackets: 2,
	}
}

func (p *quichePacer) SentPacket(sendTime monotime.Time, size protocol.ByteCount, rate Bandwidth) {
	// quiche 7980e3d5d89a: pacing_sender.cc:77-81, unpaced burst tokens.
	if p.burstTokens > 0 {
		p.burstTokens--
		p.idealNextSend = 0
		p.pacingLimited = false
		return
	}
	// quiche 7980e3d5d89a: pacing_sender.cc:87-108, packet lumps.
	if !p.pacingLimited || p.lumpyTokens == 0 {
		p.lumpyTokens = max(1, p.maxLumpPackets)
	}
	p.lumpyTokens--
	// quiche 7980e3d5d89a: pacing_sender.cc:109-117, retain the ideal
	// schedule after a late wakeup. The experiment caps catch-up debt at 4ms.
	if !p.pacingLimited || p.idealNextSend.IsZero() {
		p.idealNextSend = max(p.idealNextSend, sendTime)
	}
	p.idealNextSend = max(p.idealNextSend, sendTime.Add(-4*time.Millisecond))
	bw := max(uint64(rate/BytesPerSecond), 1)
	ns := (uint64(size)*1e9 + bw - 1) / bw
	p.idealNextSend = p.idealNextSend.Add(time.Duration(ns))
	p.pacingLimited = true
}

func (p *quichePacer) Budget(now monotime.Time, rate Bandwidth) protocol.ByteCount {
	if p.burstTokens > 0 {
		return protocol.ByteCount(p.burstTokens) * p.maxDatagramSize
	}
	// quiche 7980e3d5d89a: pacing_sender.cc:146-156, lumps and 1ms allowance.
	if p.idealNextSend.IsZero() || p.lumpyTokens > 0 {
		return protocol.ByteCount(max(1, p.lumpyTokens)) * p.maxDatagramSize
	}
	allowance := now.Add(time.Millisecond).Sub(p.idealNextSend)
	if allowance < 0 {
		return 0
	}
	return min(p.maxBurstSize(rate), p.maxDatagramSize+p.timeScaledBandwidth(allowance, rate))
}

func (p *quichePacer) maxBurstSize(rate Bandwidth) protocol.ByteCount {
	return max(
		p.timeScaledBandwidth(5*time.Millisecond, rate),
		2*p.maxDatagramSize,
	)
}

// timeScaledBandwidth calculates the number of bytes that may be sent within
// a given duration, based on the current pacing rate.
// It caps the scaled value to the maximum allowed burst and handles overflows.
func (p *quichePacer) timeScaledBandwidth(d time.Duration, rate Bandwidth) protocol.ByteCount {
	bw := uint64(rate / BytesPerSecond)
	if bw == 0 {
		return 0
	}
	const nsPerSecond = 1e9
	ns := uint64(d)
	maxBurst := maxBurstSizePackets * p.maxDatagramSize
	var scaled protocol.ByteCount
	if ns > math.MaxUint64/bw {
		scaled = maxBurst
	} else {
		scaled = protocol.ByteCount(bw * ns / nsPerSecond)
	}
	return scaled
}

// TimeUntilSend returns when the next packet should be sent.
// It returns zero if a packet can be sent immediately.
func (p *quichePacer) TimeUntilSend(rate Bandwidth) monotime.Time {
	if p.burstTokens > 0 || p.lumpyTokens > 0 || p.idealNextSend.IsZero() {
		return 0
	}
	return p.idealNextSend.Add(-time.Millisecond)
}

func (p *quichePacer) SetMaxDatagramSize(s protocol.ByteCount) {
	p.maxDatagramSize = s
}
