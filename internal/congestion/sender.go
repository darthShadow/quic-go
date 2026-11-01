package congestion

import (
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
	"github.com/quic-go/quic-go/qlogwriter"
)

// SenderConfig selects the sender and its connection-local BBR loss policy.
// Zero values preserve the upstream sender and BBR's 2 percent loss threshold.
type SenderConfig struct {
	BBRv3                bool
	NoLowerBounds        bool
	LossThresholdPercent uint8
}

// NewSender constructs the congestion sender for a path.
func NewSender(clock Clock, rtt *utils.RTTStats, stats *utils.ConnectionStats, size protocol.ByteCount, qlogger qlogwriter.Recorder, config SenderConfig) SendAlgorithmWithDebugInfos {
	if config.BBRv3 {
		b := NewBBR3Sender(clock, rtt, stats, size, 32*size, protocol.MaxCongestionWindowPackets*size, config)
		b.qlogger = qlogger
		b.maybeQlogStateChange()
		return b
	}
	return NewCubicSender(clock, rtt, stats, size, true, qlogger)
}

// SenderName reports the sender implementation used by NewSender.
func SenderName(sender SendAlgorithmWithDebugInfos) string {
	if _, ok := sender.(*bbr3Sender); ok {
		return "bbr3"
	}
	return "cubic"
}
