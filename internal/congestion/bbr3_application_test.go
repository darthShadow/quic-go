package congestion

import (
	"fmt"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func TestBBR3PacerChargesOnlyAdmittedPackets(t *testing.T) {
	p := newBBR3TestPath(utils.NewRTTStats())
	b := p.sender.(*bbr3Sender)
	b.pacingRate = 100
	b.inflight = 1200
	b.pacer.burstTokens = 0
	b.pacer.lumpyTokens = 0
	b.pacer.pacingLimited = true
	b.pacer.idealNextSend = p.clock.Now().Add(300 * time.Millisecond)
	for pn := protocol.PacketNumber(1); pn <= 6; pn++ {
		if b.HasPacingBudget(p.clock.Now()) {
			t.Fatal("fixture has pacing budget")
		}
		b.OnPacketSent(p.clock.Now(), 1248, pn, 48, true)
		p.clock.Advance(40 * time.Millisecond)
	}
	b.OnPacketSent(p.clock.Now(), 2526, 7, 1326, true)
	p.clock.Advance(bbr3TestRTT)
	if !b.HasPacingBudget(p.clock.Now()) {
		t.Fatalf("no pacing budget after one RTT: next send in %v", b.TimeUntilSend(2526).Sub(p.clock.Now()))
	}
}

func TestBBR3PacerChargesAdmittedPackets(t *testing.T) {
	p := newBBR3TestPath(utils.NewRTTStats())
	b := p.sender.(*bbr3Sender)
	b.inflight = 1200
	b.pacer.burstTokens = 0
	b.pacer.lumpyTokens = 0
	b.pacingRate = 100
	now := p.clock.Now()
	if !b.HasPacingBudget(now) {
		t.Fatal("fixture has no pacing budget")
	}
	b.OnPacketSent(now, 2526, 1, 1326, true)
	if got, want := b.pacer.idealNextSend, now.Add(13260*time.Millisecond); got != want {
		t.Fatalf("admitted packet schedule=%v, want %v", got, want)
	}
}

func TestBBR3AppLimitedSignal(t *testing.T) {
	for _, inflight := range []uint64{0, 2400} {
		t.Run(fmt.Sprint(inflight), func(t *testing.T) {
			p := newBBR3TestPath(utils.NewRTTStats())
			b := p.sender.(*bbr3Sender)
			b.delivered, b.inflight = 4800, inflight
			b.OnAppLimited(protocol.ByteCount(inflight))
			if got, want := b.appLimited, max(4800+inflight, 1); got != want {
				t.Fatalf("app-limited mark=%d, want %d", got, want)
			}
		})
	}
	p := newBBR3TestPath(utils.NewRTTStats())
	b := p.sender.(*bbr3Sender)
	b.OnAppLimited(0)
	if b.appLimited != 1 {
		t.Fatalf("empty mark=%d, want 1", b.appLimited)
	}
}

func TestBBR3AppLimitedSamples(t *testing.T) {
	p := newBBR3TestPath(utils.NewRTTStats())
	b := p.sender.(*bbr3Sender)
	p.rttStats.UpdateRTT(bbr3TestRTT, 0)
	b.delivered = 4800
	b.inflight = 2400
	b.firstTxTime, b.deliveredTime = p.clock.Now(), p.clock.Now()
	b.OnAppLimited(2400)
	const bandwidth = uint64(1_000_000)
	for pn := protocol.PacketNumber(1); pn <= 5; pn++ {
		// Age the old filter slot to expose whether a low sample replaces it.
		b.bwHi = [2]uint64{bandwidth, 0}
		b.fullBw, b.fullBwCnt = bandwidth, 0
		b.OnPacketSent(p.clock.Now(), 3600, pn, 1200, true)
		p.clock.Advance(bbr3TestRTT)
		b.OnPacketAcked(pn, 1200, 3600, p.clock.Now())
		b.GetCongestionWindow()
		wantLimited := pn <= 3 // Equality with the mark remains app-limited.
		if b.rs.isAppLimited != wantLimited {
			t.Fatalf("packet %d app-limited=%t, want %t", pn, b.rs.isAppLimited, wantLimited)
		}
		if wantLimited {
			if b.bwHi[1] != 0 || b.fullBwCnt != 0 {
				t.Fatalf("limited sample enters filters: bw=%v, full count=%d", b.bwHi, b.fullBwCnt)
			}
		} else if b.bwHi[1] == 0 || b.fullBwCnt != 1 {
			t.Fatalf("unlimited sample ignored: bw=%v, full count=%d", b.bwHi, b.fullBwCnt)
		}
	}
}
