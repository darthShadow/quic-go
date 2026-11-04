package quic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

func TestConfigACKDecimation(t *testing.T) {
	if populateConfig(nil).ACKDecimation {
		t.Fatal("ACK decimation is enabled by default")
	}
	c := &Config{ACKDecimation: true}
	if !populateConfig(c).ACKDecimation || !c.Clone().ACKDecimation {
		t.Fatal("config population or Clone loses ACKDecimation")
	}
}

func TestACKDecimationConnection(t *testing.T) {
	for _, perspective := range []protocol.Perspective{protocol.PerspectiveClient, protocol.PerspectiveServer} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", perspective, enabled), func(t *testing.T) {
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				c := &Conn{ctx: ctx, ctxCancel: cancel, config: populateConfig(&Config{ACKDecimation: enabled}), perspective: perspective, logger: utils.DefaultLogger}
				c.preSetup()
				c.rttStats.UpdateRTT(40*time.Millisecond, 0)
				now := monotime.Time(time.Second)
				for n := 1; n <= 104; n++ {
					if err := c.receivedPacketHandler.ReceivedPacket(protocol.PacketNumber(n), protocol.ECT0, protocol.Encryption1RTT, now, true); err != nil {
						t.Fatal(err)
					}
					want := n%2 == 0
					if enabled && n > 100 {
						want = (n-100)%4 == 0
					}
					if got := c.receivedPacketHandler.GetAckFrame(protocol.Encryption1RTT, now, true) != nil; got != want {
						t.Fatalf("packet %d ACK=%t, want %t", n, got, want)
					}
					if n == 101 && enabled {
						if got := c.receivedPacketHandler.GetAlarmTimeout().Sub(now); got != 10*time.Millisecond {
							t.Fatalf("ACK delay=%s, want 10ms", got)
						}
					}
				}
			})
		}
	}
}
