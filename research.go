//go:build shunpike_research

package quic

import (
	"slices"
	"sync"
	"time"

	"github.com/quic-go/quic-go/internal/congestion"
)

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
	out := map[string]any{"congestion_window_cap": congestion.BenchCongestionWindowCap()}
	benchMTUMutex.Lock()
	out["mtu_fallbacks"] = len(benchMTUFallbacks)
	out["mtu_fallback_events"] = slices.Clone(benchMTUFallbacks)
	benchMTUMutex.Unlock()
	return out
}

// BenchBBRTuning returns validated effective research controls as decimal strings.
// Invalid controls panic, as they do when constructing a BBR sender.
func BenchBBRTuning() map[string]any { return congestion.BenchBBRTuning() }
