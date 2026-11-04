package congestion

import (
	"testing"

	"github.com/quic-go/quic-go/internal/utils"
)

func TestBBRLossThresholdBounds(t *testing.T) {
	for _, percent := range []uint8{0, 1, 100} {
		b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000, SenderConfig{LossThresholdPercent: percent})
		high, _ := b.isInflightTooHigh(&b3RS{lost: 10000, txInFlight: 10000})
		if high != (percent != 100) {
			t.Errorf("threshold %d: full loss high=%t", percent, high)
		}
		if percent == 100 {
			b.inflightHiFromLostPkt(&b3RS{lost: 1200, txInFlight: 10000}, 1200)
		}
	}
}
