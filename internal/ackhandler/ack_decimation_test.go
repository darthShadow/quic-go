package ackhandler

import (
	"fmt"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func TestACKDecimationCadence(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			h := NewReceivedPacketHandler(utils.DefaultLogger)
			if enabled {
				h.EnableACKDecimation(utils.NewRTTStats())
			}
			now := monotime.Time(time.Second)
			for n := 1; n <= 120; n++ {
				if err := h.ReceivedPacket(protocol.PacketNumber(n), protocol.ECT0, protocol.Encryption1RTT, now, true); err != nil {
					t.Fatal(err)
				}
				want := n%2 == 0
				if enabled && n > 100 {
					want = (n-100)%4 == 0
				}
				if got := h.GetAckFrame(protocol.Encryption1RTT, now, true) != nil; got != want {
					t.Fatalf("packet %d ACK=%t, want %t", n, got, want)
				}
			}
			if err := h.ReceivedPacket(122, protocol.ECT0, protocol.Encryption1RTT, now, true); err != nil {
				t.Fatal(err)
			}
			if h.GetAckFrame(protocol.Encryption1RTT, now, true) == nil {
				t.Fatal("gap does not trigger an immediate ACK")
			}
		})
	}
}

func TestACKDecimationCountsAllPackets(t *testing.T) {
	h := NewReceivedPacketHandler(utils.DefaultLogger)
	h.EnableACKDecimation(utils.NewRTTStats())
	now := monotime.Time(time.Second)
	for n := 1; n <= 104; n++ {
		if err := h.ReceivedPacket(protocol.PacketNumber(n), protocol.ECT0, protocol.Encryption1RTT, now, n > 100); err != nil {
			t.Fatal(err)
		}
		if got, want := h.GetAckFrame(protocol.Encryption1RTT, now, true) != nil, n == 104; got != want {
			t.Fatalf("packet %d ACK=%t, want %t", n, got, want)
		}
	}
}

func TestACKDecimationDelay(t *testing.T) {
	for _, rtt := range []time.Duration{0, 40 * time.Millisecond, 200 * time.Millisecond} {
		t.Run(rtt.String(), func(t *testing.T) {
			h := NewReceivedPacketHandler(utils.DefaultLogger)
			stats := utils.NewRTTStats()
			h.EnableACKDecimation(stats)
			if rtt > 0 {
				stats.UpdateRTT(rtt, 0)
				stats.UpdateRTT(2*rtt, 0) // The bound uses min RTT, not the latest sample.
			}
			now := monotime.Time(time.Second)
			for n := 1; n <= 103; n++ {
				if err := h.ReceivedPacket(protocol.PacketNumber(n), protocol.ECT0, protocol.Encryption1RTT, now, true); err != nil {
					t.Fatal(err)
				}
				if n <= 100 {
					h.GetAckFrame(protocol.Encryption1RTT, now, true)
					continue
				}
				want := MaxAckDelay
				if rtt > 0 {
					want = min(want, rtt/4)
				}
				if got := h.GetAlarmTimeout().Sub(monotime.Time(time.Second)); got != want {
					t.Fatalf("packet %d alarm=%s, want %s", n, got, want)
				}
				now = now.Add(time.Millisecond)
			}
		})
	}
}
