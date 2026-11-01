package quic

import (
	"github.com/quic-go/quic-go/internal/congestion"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

// CongestionControl selects the path's congestion sender.
type CongestionControl uint8

const (
	// CongestionControlDefault preserves upstream's Reno-mode cubicSender.
	CongestionControlDefault CongestionControl = iota
	// CongestionControlBBRv3 selects the Silo BBRv3 port.
	CongestionControlBBRv3
)

func (c *Config) congestionConfig() congestion.SenderConfig {
	if c == nil {
		return congestion.SenderConfig{}
	}
	return congestion.SenderConfig{BBRv3: c.CongestionControl == CongestionControlBBRv3, NoLowerBounds: c.BBRNoLowerBounds, LossThresholdPercent: c.BBRLossThresholdPercent}
}

// CongestionControlName returns "bbr3" or "cubic" using the same constructor as
// live and migrated paths. A nil config selects the upstream sender. Validate
// unsupported Config values through the connection entry point before use.
func CongestionControlName(config *Config) string {
	sender := congestion.NewSender(congestion.DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, protocol.MinInitialPacketSize, nil, config.congestionConfig())
	return congestion.SenderName(sender)
}
