//go:build shunpike_research

package quic

import (
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/internal/congestion"
)

func researchGapLimit() int {
	text := os.Getenv("QG_MAX_GAPS")
	if text == "" {
		return 0
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 {
		panic("invalid QG_MAX_GAPS")
	}
	return n
}

var (
	benchGapHigh           atomic.Uint64
	benchGapAbort          atomic.Pointer[byteInterval]
	benchReceiveQueueDrops atomic.Uint64
)

func observeGapCount(n uint64) {
	for old := benchGapHigh.Load(); n > old; old = benchGapHigh.Load() {
		if benchGapHigh.CompareAndSwap(old, n) {
			break
		}
	}
}
func recordGapAbort(gap byteInterval) { benchGapAbort.Store(&gap) }
func recordReceiveQueueDrop()         { benchReceiveQueueDrops.Add(1) }

type benchMTUFallback struct {
	UnixNS int64  `json:"unix_ns"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Reason string `json:"reason"`
}

var (
	benchMTUMutex     sync.Mutex
	benchMTUFallbacks []benchMTUFallback
)

func recordMTUFallback(before int, reason string) {
	benchMTUMutex.Lock()
	defer benchMTUMutex.Unlock()
	benchMTUFallbacks = append(benchMTUFallbacks, benchMTUFallback{UnixNS: time.Now().UnixNano(), Before: before, After: 1200, Reason: reason})
}

// BenchSnapshot reports process-wide counters for research runs. It returns an
// independent copy of MTU events; counters are never compiled in default builds.
func BenchSnapshot() map[string]any {
	out := map[string]any{"congestion_window_cap": congestion.BenchCongestionWindowCap(), "receive_queue_drops": benchReceiveQueueDrops.Load(), "gap_high_water": benchGapHigh.Load(), "gap_abort": false, "oldest_missing_start": int64(0), "oldest_missing_end": int64(0)}
	if gap := benchGapAbort.Load(); gap != nil {
		out["gap_abort"] = true
		out["oldest_missing_start"] = int64(gap.Start)
		out["oldest_missing_end"] = int64(gap.End)
	}
	benchMTUMutex.Lock()
	out["mtu_fallbacks"] = len(benchMTUFallbacks)
	out["mtu_fallback_events"] = slices.Clone(benchMTUFallbacks)
	benchMTUMutex.Unlock()
	return out
}

// BenchBBRTuning returns validated effective research controls as decimal strings.
// Invalid controls panic, as they do when constructing a BBR sender.
func BenchBBRTuning() map[string]any { return congestion.BenchBBRTuning() }
