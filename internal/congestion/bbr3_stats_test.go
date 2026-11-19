package congestion

import (
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/utils"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/testutils/events"
)

func TestBBRConnectionStats(t *testing.T) {
	stats := &utils.ConnectionStats{}
	clock := new(mockClock)
	clock.Advance(time.Second)
	recorder := &events.Recorder{}
	b := NewSender(clock, utils.NewRTTStats(), stats, 1200, recorder, SenderConfig{BBRv3: true})
	checkWindow := func() {
		t.Helper()
		want := uint64(b.GetCongestionWindow())
		if got := stats.CongestionWindow.Load(); got != want {
			t.Errorf("CongestionWindow = %d, want %d", got, want)
		}
	}
	checkWindow()
	b.OnPacketSent(clock.Now(), 0, 1, 1200, true)
	b.OnPacketSent(clock.Now(), 1200, 2, 1200, true)
	b.OnCongestionEvent(1, 1200, 2400)
	checkWindow()
	if got := stats.PacketsLost.Load(); got != 1 {
		t.Errorf("PacketsLost = %d, want 1", got)
	}
	if got := stats.BytesLost.Load(); got != 1200 {
		t.Errorf("BytesLost = %d, want 1200", got)
	}
	clock.Advance(10 * time.Millisecond)
	b.OnPacketAcked(2, 1200, 1200, clock.Now())
	checkWindow()
	b.OnSpuriousLoss(1)
	checkWindow()
	states := recorder.Events(qlog.CongestionStateUpdated{})
	if len(states) < 2 {
		t.Fatalf("congestion events = %v, want slow start and recovery", states)
	}
	if got := states[0].(qlog.CongestionStateUpdated).State; got != qlog.CongestionStateSlowStart {
		t.Errorf("initial qlog state = %v, want slow start", got)
	}
	if got := states[1].(qlog.CongestionStateUpdated).State; got != qlog.CongestionStateRecovery {
		t.Errorf("loss qlog state = %v, want recovery", got)
	}
}
