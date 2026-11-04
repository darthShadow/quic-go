package ackhandler

import (
	"testing"

	"github.com/quic-go/quic-go/internal/congestion"
	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func newConfiguredSentPacketHandler(config congestion.SenderConfig) *sentPacketHandler {
	return NewSentPacketHandler(0, 1200, utils.NewRTTStats(), &utils.ConnectionStats{}, false, false, nil, protocol.PerspectiveClient, nil, utils.DefaultLogger, config).(*sentPacketHandler)
}

func TestDefaultConfigSelectsUpstreamSender(t *testing.T) {
	h := newConfiguredSentPacketHandler(congestion.SenderConfig{})
	for _, migrated := range []bool{false, true} {
		if migrated {
			h.MigratedPath(monotime.Now(), 1252)
		}
		if got := congestion.SenderName(h.congestion); got != "cubic" {
			t.Fatalf("migrated=%t sender = %s, want cubic", migrated, got)
		}
	}
}

func TestSenderIdentityBBRv3(t *testing.T) {
	h := newConfiguredSentPacketHandler(congestion.SenderConfig{BBRv3: true, NoLowerBounds: true, LossThresholdPercent: 5})
	for _, migrated := range []bool{false, true} {
		if migrated {
			h.MigratedPath(monotime.Now(), 1252)
		}
		if got := congestion.SenderName(h.congestion); got != "bbr3" {
			t.Fatalf("sender = %s, want bbr3", got)
		}
	}
}
