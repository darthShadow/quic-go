package ackhandler

import (
	"slices"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
	"github.com/quic-go/quic-go/internal/wire"

	"github.com/stretchr/testify/require"
)

func TestProbeNow(t *testing.T) {
	start := monotime.Now()
	newHandler := func(t *testing.T) (*sentPacketHandler, *[]protocol.ByteCount) {
		t.Helper()
		rtt := utils.NewRTTStats()
		rtt.UpdateRTT(350*time.Millisecond, 0)
		h := NewSentPacketHandler(0, 1200, rtt, &utils.ConnectionStats{}, true, false, nil, protocol.PerspectiveServer, nil, utils.DefaultLogger).(*sentPacketHandler)
		h.DropPackets(protocol.EncryptionInitial, start)
		h.DropPackets(protocol.EncryptionHandshake, start)
		var lost []protocol.ByteCount
		for i := range 6 {
			frame := &wire.StreamFrame{StreamID: 0, Offset: protocol.ByteCount(i), Data: []byte{byte(i)}}
			pn := h.PopPacketNumber(protocol.PacketNumberSpaceAppData)
			require.Equal(t, protocol.PacketNumber(i), pn)
			h.SentPacket(start, pn, protocol.InvalidPacketNumber, []StreamFrame{{Frame: frame, Handler: &customFrameHandler{onLost: func(f wire.Frame) {
				lost = append(lost, f.(*wire.StreamFrame).Offset)
			}}}}, nil, protocol.Encryption1RTT, protocol.ECNNon, 1200, false, false)
		}
		return h, &lost
	}
	registerPings := func(h *sentPacketHandler, now monotime.Time) protocol.PacketNumber {
		var pn protocol.PacketNumber
		for range 2 {
			pn = h.PopPacketNumber(protocol.PacketNumberSpaceAppData)
			h.SentPacket(now, pn, protocol.InvalidPacketNumber, nil, []Frame{{Frame: &wire.PingFrame{}, Handler: &customFrameHandler{}}}, protocol.Encryption1RTT, protocol.ECNNon, 30, false, false)
		}
		return pn
	}
	packetNumbers := func(h *sentPacketHandler) []protocol.PacketNumber {
		var numbers []protocol.PacketNumber
		for pn := range h.hist.Packets() {
			numbers = append(numbers, pn)
		}
		return numbers
	}

	for _, row := range []string{"ptoCount 0", "timeout probes pending", "handshake unconfirmed", "no outstanding data", "only 0-RTT outstanding"} {
		t.Run(row, func(t *testing.T) {
			h, lost := newHandler(t)
			h.ptoCount = 2
			switch row {
			case "ptoCount 0":
				h.ptoCount = 0
			case "timeout probes pending":
				h.numProbesToSend = 2
			case "handshake unconfirmed":
				h.handshakeConfirmed = false
			case "no outstanding data":
				_, err := h.ReceivedAck(&wire.AckFrame{AckRanges: ackRanges(0, 1, 2, 3, 4, 5)}, protocol.Encryption1RTT, start.Add(350*time.Millisecond))
				require.NoError(t, err)
				h.ptoCount = 2
			case "only 0-RTT outstanding":
				for _, p := range h.hist.Packets() {
					p.EncryptionLevel = protocol.Encryption0RTT
				}
			}
			h.setLossDetectionTimer(start)
			deadline, history, inFlight, pto := h.GetLossDetectionTimeout(), packetNumbers(h), h.bytesInFlight, h.ptoCount
			pn, _ := h.PeekPacketNumber(protocol.PacketNumberSpaceAppData)
			require.False(t, h.PrepareProbeNow(start.Add(100*time.Millisecond)))
			require.Equal(t, deadline, h.GetLossDetectionTimeout())
			require.Equal(t, history, packetNumbers(h))
			require.Equal(t, inFlight, h.bytesInFlight)
			require.Equal(t, pto, h.ptoCount)
			nextPN, _ := h.PeekPacketNumber(protocol.PacketNumberSpaceAppData)
			require.Equal(t, pn, nextPN)
			require.Empty(t, *lost)
		})
	}

	t.Run("ptoCount 2 preserves deadline and history", func(t *testing.T) {
		h, lost := newHandler(t)
		h.ptoCount = 2
		h.setLossDetectionTimer(start)
		deadline, history, inFlight := h.GetLossDetectionTimeout(), packetNumbers(h), h.bytesInFlight
		cwnd, rtt := h.congestion.GetCongestionWindow(), h.rttStats.SmoothedRTT()
		now := start.Add(100 * time.Millisecond)
		require.True(t, h.PrepareProbeNow(now))
		require.Equal(t, deadline, h.probeNowDeadline)
		require.Equal(t, history, packetNumbers(h))
		require.Equal(t, inFlight, h.bytesInFlight)
		pn := registerPings(h, now)
		require.Equal(t, protocol.PacketNumber(7), pn, "preparation skips no packet number")
		require.Equal(t, history, packetNumbers(h)[:len(history)])
		require.Equal(t, uint32(2), h.ptoCount)
		require.Zero(t, h.numProbesToSend)
		require.Empty(t, *lost)
		require.Equal(t, cwnd, h.congestion.GetCongestionWindow())
		require.Equal(t, rtt, h.rttStats.SmoothedRTT())
		require.Equal(t, deadline, h.GetLossDetectionTimeout())
		require.True(t, h.PrepareProbeNow(now.Add(100*time.Millisecond)))
		require.Equal(t, deadline, h.probeNowDeadline, "repeated calls keep the earlier deadline")
	})

	t.Run("differential lost PINGs preserve timeout frames", func(t *testing.T) {
		a, lostA := newHandler(t)
		b, lostB := newHandler(t)
		for _, h := range []*sentPacketHandler{a, b} {
			h.ptoCount = 2
			h.setLossDetectionTimer(start)
		}
		deadline := b.GetLossDetectionTimeout()
		require.True(t, a.PrepareProbeNow(start.Add(100*time.Millisecond)))
		registerPings(a, start.Add(100*time.Millisecond))
		require.Equal(t, deadline, a.GetLossDetectionTimeout())
		for _, h := range []*sentPacketHandler{a, b} {
			require.NoError(t, h.OnLossDetectionTimeout(deadline))
			require.Zero(t, h.probeNowDeadline)
			for range 2 {
				require.True(t, h.QueueProbePacket(protocol.PacketNumberSpaceAppData))
			}
		}
		require.Equal(t, []protocol.ByteCount{0, 1}, *lostA)
		require.Equal(t, *lostB, *lostA)
		require.Equal(t, b.ptoCount, a.ptoCount)
		require.True(t, slices.Equal(packetNumbers(b), packetNumbers(a)[:len(packetNumbers(b))]))
	})

	t.Run("ACK of a PING clears deadline and backoff", func(t *testing.T) {
		h, _ := newHandler(t)
		h.ptoCount = 2
		h.setLossDetectionTimer(start)
		now := start.Add(100 * time.Millisecond)
		require.True(t, h.PrepareProbeNow(now))
		pn := registerPings(h, now)
		acked, err := h.ReceivedAck(&wire.AckFrame{AckRanges: ackRanges(pn)}, protocol.Encryption1RTT, now.Add(350*time.Millisecond))
		require.NoError(t, err)
		require.True(t, acked)
		require.Zero(t, h.ptoCount)
		require.Zero(t, h.probeNowDeadline)
	})
}
