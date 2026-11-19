package congestion

import (
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func TestQuichePacerBursts(t *testing.T) {
	b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	b.pacingRate = 12500000
	now := monotime.Time(time.Second)
	if got := b.pacer.Budget(now, b.effectivePacingRate()); got != 10*1200 {
		t.Fatalf("initial burst=%d, want 12000", got)
	}
	for n := protocol.PacketNumber(1); b.HasPacingBudget(now); n++ {
		b.OnPacketSent(now, 12000+protocol.ByteCount(n)*1200, n, 1200, true)
	}
	now = now.Add(100 * time.Millisecond)
	b.OnPacketSent(now, 1200, 100, 1200, true)
	if got := b.pacer.Budget(now, b.effectivePacingRate()); got != 9*1200 {
		t.Fatalf("idle burst=%d, want 10800", got)
	}
	if got := b.TimeUntilSend(1200); got != 0 {
		t.Fatalf("idle deadline=%s, want immediate", got.Sub(now))
	}
	b.cwnd = 4 * 1200
	b.OnPacketSent(now, 1200, 101, 1200, true)
	if got := b.pacer.Budget(now, b.effectivePacingRate()); got != 3*1200 {
		t.Fatalf("cwnd-bounded idle burst=%d, want 3600", got)
	}
}

func TestQuichePacerLumps(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cwnd, bw uint64
		wantLump bool
	}{
		{"two packets", 38400, 600000, true},
		{"low bandwidth", 38400, 100000, false},
		{"quarter cwnd", 4800, 600000, false},
		{"cwnd limited", 13200, 600000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
			b.cwnd, b.pacingRate, b.bwHi = tc.cwnd, tc.bw, [2]uint64{tc.bw, 0}
			now := monotime.Time(time.Second)
			for n := protocol.PacketNumber(1); n <= 11; n++ {
				b.OnPacketSent(now, 1200+protocol.ByteCount(n)*1200, n, 1200, true)
			}
			if got := b.TimeUntilSend(14400); (got == 0) != tc.wantLump {
				t.Fatalf("extra lump=%t, want %t (deadline %s)", got == 0, tc.wantLump, got.Sub(now))
			}
		})
	}
}

func TestQuichePacerCatchup(t *testing.T) {
	b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	b.pacingRate = 12500000
	p := b.pacer
	rate := b.effectivePacingRate()
	now := monotime.Time(time.Second)
	for p.Budget(now, rate) >= 1200 {
		p.SentPacket(now, 1200, rate)
	}
	now = p.TimeUntilSend(rate).Add(8 * time.Millisecond)
	if got := p.Budget(now, rate); got <= 25000 {
		t.Fatalf("late budget=%d, want above 2ms (25000)", got)
	}
	sent := 0
	for p.Budget(now, rate) >= 1200 {
		p.SentPacket(now, 1200, rate)
		sent += 1200
		if sent > 75000 {
			t.Fatalf("catchup=%d, exceeds 6ms including packet rounding", sent)
		}
	}
	if next := p.TimeUntilSend(rate); next.After(now.Add(time.Millisecond)) {
		t.Fatalf("next deadline=%s, want within 1ms", next.Sub(now))
	}
}

func TestQuichePacerDebtCap(t *testing.T) {
	b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	p := b.pacer
	rate := Bandwidth(1200000) * BytesPerSecond
	now := monotime.Time(time.Second)
	for p.Budget(now, rate) >= 1200 {
		p.SentPacket(now, 1200, rate)
	}
	now = now.Add(100 * time.Millisecond)
	p.SentPacket(now, 1200, rate)
	p.SentPacket(now, 1200, rate) // Exhaust the two-packet lump to expose the deadline.
	if got, want := p.TimeUntilSend(rate), now.Add(-3*time.Millisecond); got != want {
		t.Fatalf("deadline debt=%s, want %s", got.Sub(now), want.Sub(now))
	}
}

func TestQuichePacerResets(t *testing.T) {
	t.Run("loss cancels burst and recovery prevents idle burst", func(t *testing.T) {
		b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
		b.pacingRate = 1200000
		now := monotime.Time(time.Second)
		b.OnPacketSent(now, 1200, 1, 1200, true)
		b.OnCongestionEvent(1, 1200, 1200)
		if got := b.pacer.Budget(now, b.effectivePacingRate()); got != 1200 {
			t.Fatalf("budget after loss=%d, want 1200", got)
		}
		b.OnPacketSent(now, 1200, 2, 1200, true)
		if got := b.pacer.Budget(now, b.effectivePacingRate()); got > 2*1200 {
			t.Fatalf("recovery idle budget=%d, want at most 2400", got)
		}
	})
	t.Run("cwnd limitation ends catchup", func(t *testing.T) {
		b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
		b.pacingRate, b.bwHi = 1200000, [2]uint64{1200000, 0}
		now := monotime.Time(time.Second)
		for n := protocol.PacketNumber(1); b.HasPacingBudget(now); n++ {
			b.OnPacketSent(now, 12000+protocol.ByteCount(n)*1200, n, 1200, true)
		}
		if b.CanSend(protocol.ByteCount(b.cwnd)) {
			t.Fatal("sender accepts a full cwnd")
		}
		now = now.Add(100 * time.Millisecond)
		b.OnPacketSent(now, 12000, 100, 1200, true)
		b.OnPacketSent(now, 13200, 101, 1200, true)
		if got := b.TimeUntilSend(13200).Sub(now); got != time.Millisecond {
			t.Fatalf("deadline after cwnd limitation=%s, want 1ms", got)
		}
	})
}

// A paced wakeup releases one lump. The lump spans 0.5 ms at the pacing rate,
// at least two packets, at most one GSO buffer and at most 0.25 cwnd.
func TestBBR3LumpSpansHalfMillisecond(t *testing.T) {
	const mss = 1452
	for _, tc := range []struct {
		name       string
		rate, cwnd uint64
		inflight   protocol.ByteCount
		want       int
	}{
		{"1 Gbit/s fills one GSO buffer", 125_000_000, 50_000_000, 2 * mss, 14},
		{"200 Mbit/s sends 0.5 ms", 25_000_000, 50_000_000, 2 * mss, 8},
		{"50 Mbit/s keeps two packets", 6_250_000, 50_000_000, 2 * mss, 2},
		{"quarter cwnd caps the lump", 25_000_000, 24 * mss, 2 * mss, 6},
		{"quarter cwnd rounds down", 25_000_000, 27 * mss, 2 * mss, 6},
		{"small cwnd keeps one packet", 25_000_000, 3 * mss, 2 * mss, 1},
		{"below 1.2 Mbit/s keeps one packet", 149_999, 50_000_000, 2 * mss, 1},
		{"at 1.2 Mbit/s keeps two packets", 150_000, 50_000_000, 2 * mss, 2},
		{"at cwnd keeps one packet", 25_000_000, 24 * mss, 24 * mss, 1},
		{"above cwnd keeps one packet", 25_000_000, 24 * mss, 25 * mss, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, mss, 32*mss, 50_000_000)
			b.cwnd, b.pacingRate, b.bwHi = tc.cwnd, tc.rate, [2]uint64{tc.rate, 0}
			b.pacer.burstTokens = 0
			pn := protocol.PacketNumber(1)
			wakeup := func(now monotime.Time) int {
				n := 0
				for ; b.HasPacingBudget(now); n++ {
					if n == 1000 { // A broken cwnd override refills the budget on every send.
						t.Fatal("pacing budget never ran out")
					}
					b.OnPacketSent(now, tc.inflight, pn, mss, true)
					pn++
				}
				return n
			}
			wakeup(monotime.Time(time.Second)) // Leave the 1 ms start allowance.
			if got := wakeup(b.TimeUntilSend(tc.inflight)); got != tc.want {
				t.Fatalf("packets per paced wakeup=%d, want %d", got, tc.want)
			}
		})
	}
}
