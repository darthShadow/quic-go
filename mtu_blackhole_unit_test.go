package quic

import (
	"syscall"
	"testing"

	"github.com/quic-go/quic-go/internal/ackhandler"
	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
)

type mtuErrorSendConn struct{ sendConn }

func (*mtuErrorSendConn) Write([]byte, uint16, protocol.ECN) error { return syscall.EMSGSIZE }

func TestPMTUErrorSignal(t *testing.T) {
	for _, tc := range []struct {
		name           string
		length         int
		gso            uint16
		probe, enabled bool
		want           protocol.ByteCount
	}{
		{"minimum", 1200, 0, false, true, 0},
		{"data", 1400, 0, false, true, 1400},
		{"minimum-GSO", 4800, 1200, false, true, 0},
		{"data-GSO", 5600, 1400, false, true, 1400},
		{"probe", 1400, 0, true, true, 0},
		{"disabled", 1400, 0, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := newSendQueue(&mtuErrorSendConn{}, tc.enabled)
			signals, ok := q.(interface {
				SendMsgSizeErrors() <-chan protocol.ByteCount
				SendMTUProbe(*packetBuffer, protocol.ECN)
			})
			if !ok {
				t.Fatal("send queue does not signal data-packet EMSGSIZE")
			}
			done := make(chan error, 1)
			go func() { done <- q.Run() }()
			buf := getLargePacketBuffer()
			buf.Data = buf.Data[:tc.length]
			if tc.probe {
				signals.SendMTUProbe(buf, protocol.ECNUnsupported)
			} else {
				q.Send(buf, tc.gso, protocol.ECNUnsupported)
			}
			q.Close()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			var got protocol.ByteCount
			select {
			case got = <-signals.SendMsgSizeErrors():
			default:
			}
			if got != tc.want {
				t.Fatalf("logical data-packet error size=%d, want %d", got, tc.want)
			}
		})
	}
}

type mtuUnitSendConn struct{ sendConn }

func (*mtuUnitSendConn) capabilities() connCapabilities { return connCapabilities{DF: true} }

func TestPMTURecoveryConfig(t *testing.T) {
	for _, tc := range []struct {
		name               string
		enabled, confirmed bool
		start, want        protocol.ByteCount
	}{
		{"enabled", true, true, 1400, 1200},
		{"disabled", false, true, 1400, 1400},
		{"handshake", true, false, 1400, 1400},
		{"minimum", true, true, 1200, 1200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rtt := utils.NewRTTStats()
			c := Conn{config: &Config{PMTUBlackHoleDetection: tc.enabled}, handshakeConfirmed: tc.confirmed, conn: &mtuUnitSendConn{}, mtuDiscoverer: newMTUDiscoverer(rtt, tc.start, 1452, nil), sentPacketHandler: ackhandler.NewSentPacketHandler(0, tc.start, rtt, &utils.ConnectionStats{}, false, false, nil, protocol.PerspectiveClient, nil, utils.DefaultLogger)}
			now := monotime.Now()
			c.recoverPathMTU(now, "EMSGSIZE")
			if got := c.mtuDiscoverer.CurrentSize(); got != tc.want {
				t.Fatalf("MTU=%d, want %d", got, tc.want)
			}
			if tc.start > tc.want && c.mtuDiscoverer.lastProbeTime != now {
				t.Fatal("fallback did not restart MTU discovery")
			}
		})
	}
}
