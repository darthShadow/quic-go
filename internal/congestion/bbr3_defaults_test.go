//go:build !shunpike_research

package congestion

import (
	"testing"

	"github.com/quic-go/quic-go/internal/utils"
)

func TestBBRDefaultsC2(t *testing.T) {
	b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	if b.tuning.probeRTTWinUS != 10000000 {
		t.Errorf("ProbeRTT interval=%d, want 10000000", b.tuning.probeRTTWinUS)
	}
	if got := b.bwToPacingRate(1000000, b3Unit); got != 1000000 {
		t.Errorf("pacing rate=%d, want 1000000", got)
	}
	b.bwHi = [2]uint64{1000000, 1000000}
	b.extraAcked = [2]uint64{1000000, 0}
	if got := b.ackAggregationCwnd(); got != 1000000 {
		t.Errorf("ACK aggregation=%d, want 1000000", got)
	}
	if b.tuning.ackWindow != 10 {
		t.Errorf("ACK window=%d, want 10", b.tuning.ackWindow)
	}
}
