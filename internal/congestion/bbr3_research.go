//go:build shunpike_research

package congestion

import (
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
)

var benchCongestionWindowCap atomic.Uint64

// BenchCongestionWindowCap reports the clamp installed in the latest sender.
func BenchCongestionWindowCap() uint64 { return benchCongestionWindowCap.Load() }

// Measurement controls select C2 unless the previous profile is explicit.
// Both sender construction and evidence use these validated effective values.
func readBBR3Config() map[string]int64 {
	profile := os.Getenv("QG_BBR_DEFAULTS")
	if profile != "" && profile != "c2" && profile != "previous" {
		panic(fmt.Sprintf("BBRv3 startup refused: QG_BBR_DEFAULTS=%q; require c2 or previous", profile))
	}
	margin, window, uncapped, probeMS := int64(0), int64(10), int64(1), int64(10000)
	if profile == "previous" {
		margin, window, uncapped, probeMS = 1, 5, 0, 5000
	}
	read := func(key string, fallback, low, high int64) int64 {
		text := os.Getenv(key)
		if text == "" {
			return fallback
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil || value < low || value > high {
			panic(fmt.Sprintf("BBRv3 startup refused: %s=%q; require integer in [%d,%d]", key, text, low, high))
		}
		return value
	}
	return map[string]int64{
		"bbr_debug":                   read("QG_BBR_DEBUG", 0, 0, 1),
		"bbr_pacing_margin_pct":       read("QG_BBR_PACING_MARGIN_PCT", margin, 0, 10),
		"bbr_ack_window_rounds":       read("QG_BBR_ACK_WINDOW_ROUNDS", window, 1, 31),
		"bbr_ack_uncapped":            read("QG_BBR_ACK_UNCAPPED", uncapped, 0, 1),
		"bbr_headroom_pct":            read("QG_BBR_HEADROOM_PCT", 15, 0, 50),
		"bbr_cwnd_gain_pct":           read("QG_BBR_CWND_GAIN_PCT", 200, 100, 300),
		"bbr_probe_rtt_win_ms":        read("QG_BBR_PROBE_RTT_WIN_MS", probeMS, 1000, 60000),
		"bbr_probe_rtt_cwnd_gain_pct": read("QG_BBR_PROBE_RTT_CWND_GAIN_PCT", 50, 1, 100),
	}
}

func readBBR3Tuning() bbr3Tuning {
	config := readBBR3Config()
	return bbr3Tuning{
		pacingMargin:  uint64(config["bbr_pacing_margin_pct"]),
		ackWindow:     uint32(config["bbr_ack_window_rounds"]),
		ackUncapped:   config["bbr_ack_uncapped"] == 1,
		headroom:      uint64(config["bbr_headroom_pct"]) * b3Unit / 100,
		cwndGain:      uint64(config["bbr_cwnd_gain_pct"]) * b3Unit / 100,
		probeRTTWinUS: config["bbr_probe_rtt_win_ms"] * 1000,
		probeRTTGain:  uint64(config["bbr_probe_rtt_cwnd_gain_pct"]) * b3Unit / 100,
	}
}

// BenchBBRTuning returns validated effective tuning as decimal strings.
// Invalid controls panic with the same startup refusal as sender construction.
func BenchBBRTuning() map[string]any {
	result := make(map[string]any)
	for key, value := range readBBR3Config() {
		result[key] = strconv.FormatInt(value, 10)
	}
	profile := os.Getenv("QG_BBR_DEFAULTS")
	if profile == "" {
		profile = "c2"
	}
	result["bbr_defaults"] = profile
	return result
}

func (b *bbr3Sender) debugState(event string) {
	if !b.dbg {
		return
	}
	// Bytes and bytes/s avoid the rounding in the human-readable 1 Hz line.
	fmt.Fprintf(os.Stderr, "bbr3_diag {\"unix_ns\":%d,\"sender\":\"%p\",\"event\":\"%s\",\"mode\":%d,\"cycle\":%d,\"inflight_hi\":%d,\"cwnd\":%d,\"bw_hi\":%d,\"pacing_rate\":%d}\n",
		time.Now().UnixNano(), b, event, b.mode, b.cycleIdx, b.inflightHi, b.cwnd, b.maxBw(), b.pacingRate)
}

func (b *bbr3Sender) debugLine() {
	if !b.dbg || b.now.Sub(b.dbgLast) < time.Second {
		return
	}
	b.dbgLast = b.now
	b.debugState("sample")
	lo := func(v uint64) string {
		if v == b3Inf {
			return "inf"
		}
		return fmt.Sprint(v / 1000)
	}
	fmt.Fprintf(os.Stderr, "bbr3 mode=%d cyc=%d bw=%.1fMB/s hi=%.1f lo=%s infl_hi=%sK infl_lo=%sK cwnd=%dK minrtt=%dus pace=%.1fMB/s full=%v lost=%d own_infl=%d qg_infl=%d npkts=%d\n",
		b.mode, b.cycleIdx, float64(b.bw())/1e6, float64(b.maxBw())/1e6, lo(b.bwLo), lo(b.inflightHi), lo(b.inflightLo),
		b.cwnd/1000, b.minRTTUS, float64(b.pacingRate)/1e6, b.fullBwReached, b.lost, b.inflight, b.dbgBIF, len(b.pkts))
}

func configureBBRResearch(b *bbr3Sender) {
	b.dbg = os.Getenv("QG_BBR_DEBUG") == "1"
	if v := os.Getenv("QG_BBR_NO_LOWER"); v != "" {
		b.noLower = true
	}
	if text := os.Getenv("QG_BBR_LOSS_THRESH"); text != "" {
		v, err := strconv.ParseFloat(text, 64)
		if err != nil || !(v >= 1 && v <= 100) {
			panic("invalid QG_BBR_LOSS_THRESH")
		}
		b.lossThreshold = uint64(float64(b3Unit) * v / 100)
	}
	benchCongestionWindowCap.Store(b.cwndClamp)
}

func init() { readBBR3Tuning() }

type bbr3Debug struct {
	dbg     bool
	dbgLast monotime.Time
	dbgBIF  uint64
}

func (b *bbr3Sender) recordDebugInflight(n protocol.ByteCount) { b.dbgBIF = uint64(n) }
