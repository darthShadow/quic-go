//go:build shunpike_research

package quic

import "github.com/quic-go/quic-go/internal/congestion"

// BenchSnapshot reports process-wide counters for research runs. Counters are
// never compiled in default builds.
func BenchSnapshot() map[string]any {
	out := map[string]any{"congestion_window_cap": congestion.BenchCongestionWindowCap()}
	return out
}

// BenchBBRTuning returns validated effective research controls as decimal strings.
// Invalid controls panic, as they do when constructing a BBR sender.
func BenchBBRTuning() map[string]any { return congestion.BenchBBRTuning() }
