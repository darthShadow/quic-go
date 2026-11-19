//go:build !shunpike_research

package congestion

import "github.com/quic-go/quic-go/internal/protocol"

type bbr3Debug struct{}

func (*bbr3Debug) recordDebugInflight(protocol.ByteCount) {}

func readBBR3Tuning() bbr3Tuning {
	return bbr3Tuning{pacingMargin: b3PacingMarginPct, ackWindow: b3ExtraAckedWinRTTs, ackUncapped: true, headroom: b3InflightHeadroom, cwndGain: b3CwndGain, probeRTTWinUS: b3ProbeRTTWinUS, probeRTTGain: b3ProbeRTTCwndGain}
}
func configureBBRResearch(*bbr3Sender) {}
func (*bbr3Debug) debugLine()          {}
func (*bbr3Debug) debugState(string)   {}
