package congestion

import (
	"fmt"
	"testing"

	"github.com/quic-go/quic-go/internal/utils"
)

func assertLossPolicy(t *testing.T, b *bbr3Sender, noLower bool, percent uint8) {
	t.Helper()
	b.mode = b3ProbeBW
	b.cycleIdx = b3ProbeCruise
	b.lossInRound = true
	b.bwLo = 10000
	b.inflightLo = 10000
	b.bwLatest = 1
	b.inflightLatest = 1
	b.adaptLowerBounds()
	if got := b.bwLo == 10000; got != noLower {
		t.Errorf("noLower = %t, want %t (bwLo=%d)", got, noLower, b.bwLo)
	}
	high, _ := b.isInflightTooHigh(&b3RS{lost: 300, txInFlight: 10000})
	if want := percent == 2 || percent == 0; high != want {
		t.Errorf("3 percent loss above threshold=%d: got %t, want %t", percent, high, want)
	}
}

func TestBBRLossResponseConfig(t *testing.T) {
	for _, noLower := range []bool{false, true} {
		for _, percent := range []uint8{2, 5} {
			t.Run(fmt.Sprintf("noLower=%t/threshold=%d", noLower, percent), func(t *testing.T) {
				first := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000, SenderConfig{NoLowerBounds: noLower, LossThresholdPercent: percent})
				secondPercent := uint8(7) - percent
				second := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000, SenderConfig{NoLowerBounds: !noLower, LossThresholdPercent: secondPercent})
				assertLossPolicy(t, first, noLower, percent)
				assertLossPolicy(t, second, !noLower, secondPercent)
				assertLossPolicy(t, first, noLower, percent)
			})
		}
	}
}
