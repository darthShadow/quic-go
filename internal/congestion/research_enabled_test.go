//go:build shunpike_research

package congestion

import (
	"testing"

	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func TestResearchIgnoresRetiredCongestionWindowCap(t *testing.T) {
	for _, value := range []string{"12000000", "24000000", "48000000", "100800000", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("QG_CWND_CAP_BYTES", value)
			b := NewSender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, nil, SenderConfig{BBRv3: true}).(*bbr3Sender)
			want := uint64(protocol.MaxCongestionWindowPackets) * 1200
			if b.cwndClamp != want {
				t.Errorf("sender clamp=%d, want %d", b.cwndClamp, want)
			}
			if got := BenchCongestionWindowCap(); got != want {
				t.Errorf("reported clamp=%d, want %d", got, want)
			}
		})
	}
}

func TestResearchControls(t *testing.T) {
	for _, key := range []string{"QG_BBR_NO_LOWER", "QG_BBR_LOSS_THRESH", "QG_BBR_DEBUG", "QG_BBR_PACING_MARGIN_PCT", "QG_BBR_ACK_WINDOW_ROUNDS", "QG_BBR_ACK_UNCAPPED", "QG_BBR_HEADROOM_PCT", "QG_BBR_CWND_GAIN_PCT", "QG_BBR_PROBE_RTT_WIN_MS", "QG_BBR_PROBE_RTT_CWND_GAIN_PCT"} {
		t.Setenv(key, "")
	}
	t.Setenv("QG_BBR_DEFAULTS", "previous")
	t.Setenv("QG_BBR_NO_LOWER", "1")
	t.Setenv("QG_BBR_LOSS_THRESH", "5")
	t.Setenv("QG_BBR_PACING_MARGIN_PCT", "3")
	b := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	if b.cwndClamp != 12000000 {
		t.Errorf("research clamp=%d, want 12000000", b.cwndClamp)
	}
	assertLossPolicy(t, b, true, 5)
	if got := b.bwToPacingRate(1000000, b3Unit); got != 970000 {
		t.Errorf("research pacing=%d, want 970000", got)
	}
	if got := BenchBBRTuning()["bbr_defaults"]; got != "previous" {
		t.Errorf("profile=%v, want previous", got)
	}
	if got := BenchCongestionWindowCap(); got != 12000000 {
		t.Errorf("reported clamp=%d, want 12000000", got)
	}
	t.Setenv("QG_BBR_DEFAULTS", "c2")
	t.Setenv("QG_BBR_PACING_MARGIN_PCT", "")
	c := NewBBR3Sender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, 38400, 12000000)
	if c.tuning.probeRTTWinUS != 10000000 || c.tuning.ackWindow != 10 || !c.tuning.ackUncapped || c.tuning.pacingMargin != 0 {
		t.Fatalf("C2 profile=%+v", c.tuning)
	}
}
