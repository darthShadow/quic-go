package congestion

import (
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

const bbr3TestRTT = 350 * time.Millisecond

type bbr3TestPacket struct {
	number protocol.PacketNumber
	sent   monotime.Time
}

// bbr3TestPath delivers every packet after one RTT and keeps the sender busy.
type bbr3TestPath struct {
	sender   SendAlgorithmWithDebugInfos
	clock    *mockClock
	rttStats *utils.RTTStats
	pending  []bbr3TestPacket
	nextPN   protocol.PacketNumber
	inflight protocol.ByteCount
}

func newBBR3TestPath(rttStats *utils.RTTStats) *bbr3TestPath {
	clock := new(mockClock)
	clock.Advance(time.Second)
	return &bbr3TestPath{
		sender:   NewSender(clock, rttStats, &utils.ConnectionStats{}, 1200, nil, SenderConfig{BBRv3: true}),
		clock:    clock,
		rttStats: rttStats,
	}
}

func (p *bbr3TestPath) advance(t *testing.T, duration time.Duration) {
	t.Helper()
	end := p.clock.Now().Add(duration)
	for p.clock.Now() < end {
		now := p.clock.Now()
		for len(p.pending) > 0 && p.pending[0].sent.Add(bbr3TestRTT) <= now {
			packet := p.pending[0]
			p.pending = p.pending[1:]
			p.rttStats.UpdateRTT(now.Sub(packet.sent), 0)
			p.sender.OnPacketAcked(packet.number, 1200, p.inflight, now)
			p.inflight -= 1200
		}
		for p.sender.CanSend(p.inflight) && p.sender.HasPacingBudget(now) {
			p.nextPN++
			p.inflight += 1200
			p.sender.OnPacketSent(now, p.inflight, p.nextPN, 1200, true)
			p.pending = append(p.pending, bbr3TestPacket{number: p.nextPN, sent: now})
		}
		next := end
		if len(p.pending) > 0 {
			next = min(next, p.pending[0].sent.Add(bbr3TestRTT))
		}
		if p.sender.CanSend(p.inflight) {
			next = min(next, p.sender.TimeUntilSend(p.inflight))
		}
		if next <= now {
			t.Fatalf("path clock stalls at %v: next=%v, inflight=%d", now, next, p.inflight)
		}
		p.clock.Advance(next.Sub(now))
	}
}

func TestBBR3MinRTTIgnoresInitialRTTSeed(t *testing.T) {
	p := newBBR3TestPath(utils.NewRTTStats())
	b := p.sender.(*bbr3Sender)
	if got := b.minRTTNow(); got != b3NoRTT {
		t.Errorf("unmeasured min RTT=%d us, want b3NoRTT (%d)", got, b3NoRTT)
	}
	if b.minRTTUS != b3NoRTT || b.probeRTTMinUS != b3NoRTT {
		t.Errorf("initial RTT filters=(%d, %d), want (%d, %d)", b.minRTTUS, b.probeRTTMinUS, b3NoRTT, b3NoRTT)
	}
	p.advance(t, bbr3TestRTT+time.Nanosecond)
	if !p.rttStats.HasMeasurement() {
		t.Fatal("normal ACK path has no RTT measurement")
	}
	want := bbr3TestRTT.Microseconds()
	if got := b.minRTTNow(); got != want {
		t.Errorf("measured min RTT=%d us, want %d", got, want)
	}
	if b.minRTTUS != want || b.probeRTTMinUS != want {
		t.Errorf("first RTT filters=(%d, %d), want (%d, %d)", b.minRTTUS, b.probeRTTMinUS, want, want)
	}
}

func TestBBR3RateSampleRequiresRTTMeasurement(t *testing.T) {
	for _, tc := range []struct {
		name         string
		measured     bool
		interval     time.Duration
		wantInterval int64
	}{
		{"unmeasured_below_seed", false, 50 * time.Millisecond, -1},
		{"unmeasured_above_seed", false, bbr3TestRTT, -1},
		{"measured_below_minimum", true, 50 * time.Millisecond, -1},
		{"measured_at_minimum", true, bbr3TestRTT, bbr3TestRTT.Microseconds()},
		{"measured_above_minimum", true, 400 * time.Millisecond, (400 * time.Millisecond).Microseconds()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rttStats := utils.NewRTTStats()
			if tc.measured {
				rttStats.UpdateRTT(bbr3TestRTT, 0)
			}
			p := newBBR3TestPath(rttStats)
			b := p.sender.(*bbr3Sender)
			p.sender.OnPacketSent(p.clock.Now(), 1200, 1, 1200, true)
			p.clock.Advance(tc.interval)
			p.sender.OnPacketAcked(1, 1200, 1200, p.clock.Now())
			p.sender.GetCongestionWindow()
			if b.rs.intervalUS != tc.wantInterval {
				t.Errorf("rate sample interval=%d us, want %d", b.rs.intervalUS, tc.wantInterval)
			}
			var wantBandwidth uint64
			if tc.wantInterval > 0 {
				wantBandwidth = mulDivUp(1200, 1000*1000, uint64(tc.wantInterval))
			}
			if b.sampleBw != wantBandwidth {
				t.Errorf("sample bandwidth=%d bytes/s, want %d", b.sampleBw, wantBandwidth)
			}
			if !tc.measured {
				want := tc.interval.Microseconds()
				if b.minRTTUS != want || b.probeRTTMinUS != want {
					t.Errorf("unmeasured RTT filters=(%d, %d), want (%d, %d)", b.minRTTUS, b.probeRTTMinUS, want, want)
				}
			}
		})
	}
}

func TestBBR3StartupGrowsAtLongRTT(t *testing.T) {
	p := newBBR3TestPath(utils.NewRTTStats())
	initialWindow := p.sender.GetCongestionWindow()
	for round := 1; round <= 10; round++ {
		p.advance(t, bbr3TestRTT)
	}
	window := p.sender.GetCongestionWindow()
	if window <= 128*initialWindow {
		t.Errorf("cwnd after ten lossless rounds=%d, want > %d", window, 128*initialWindow)
	}
	if !p.sender.InSlowStart() {
		t.Errorf("Startup ends after ten lossless rounds: cwnd=%d, fullBwReached=%t", window, p.sender.(*bbr3Sender).fullBwReached)
	}
}

func TestBBR3SenderWindowCap(t *testing.T) {
	rttStats := utils.NewRTTStats()
	rttStats.UpdateRTT(bbr3TestRTT, 0)
	p := newBBR3TestPath(rttStats)
	const want = protocol.ByteCount(42000 * 1200)
	// Ten seconds covers growth to the clamp before the first ProbeRTT.
	p.advance(t, 10*time.Second-time.Nanosecond)
	if got := p.sender.GetCongestionWindow(); got != want {
		t.Fatalf("NewSender BBRv3 window ceiling=%d, want %d", got, want)
	}
	if p.sender.CanSend(want) || !p.sender.CanSend(want-1200) {
		t.Fatal("CanSend does not enforce the 42,000-packet ceiling")
	}
}
