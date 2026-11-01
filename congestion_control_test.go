package quic

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/handshake"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"

	"go.uber.org/mock/gomock"
)

func TestCongestionControlName(t *testing.T) {
	for _, tc := range []struct {
		config *Config
		name   string
	}{
		{nil, "cubic"}, {&Config{}, "cubic"}, {&Config{CongestionControl: CongestionControlBBRv3}, "bbr3"},
	} {
		if got := CongestionControlName(tc.config); got != tc.name {
			t.Errorf("sender = %s, want %s", got, tc.name)
		}
	}
}

func TestSiloConfigValidation(t *testing.T) {
	for _, p := range []uint8{0, 1, 2, 5, 100, 101, 255} {
		err := validateConfig(&Config{BBRLossThresholdPercent: p})
		if (err != nil) != (p > 100) {
			t.Errorf("loss threshold %d: error=%v", p, err)
		}
	}
	if err := validateConfig(&Config{CongestionControl: CongestionControl(255)}); err == nil {
		t.Error("unknown congestion control accepted")
	}
}

func TestBBRConnectionDisablesECN(t *testing.T) {
	for _, server := range []bool{false, true} {
		for _, bbr := range []bool{false, true} {
			name := "client"
			if server {
				name = "server"
			}
			if bbr {
				name += "/bbr3"
			} else {
				name += "/upstream"
			}
			t.Run(name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				transport := NewMockSendConn(ctrl)
				transport.EXPECT().capabilities().Return(connCapabilities{ECN: true}).AnyTimes()
				transport.EXPECT().LocalAddr().Return(&net.UDPAddr{Port: 1234}).AnyTimes()
				transport.EXPECT().RemoteAddr().Return(&net.UDPAddr{Port: 4321}).AnyTimes()
				config := populateConfig(&Config{DisablePathMTUDiscovery: true})
				if bbr {
					config.CongestionControl = CongestionControlBBRv3
				}
				runner := NewMockConnRunner(ctrl)
				id := protocol.ParseConnectionID([]byte("testconn"))
				var conn *Conn
				if server {
					ctx, cancel := context.WithCancelCause(t.Context())
					defer cancel(nil)
					conn = newConnection(ctx, cancel, transport, runner, id, nil, protocol.ConnectionID{}, protocol.ConnectionID{}, id,
						&protocol.DefaultConnectionIDGenerator{}, newStatelessResetter(nil), config, &tls.Config{},
						handshake.NewTokenGenerator(handshake.TokenProtectorKey{}), false, time.Millisecond, nil, utils.DefaultLogger, protocol.Version1).Conn
				} else {
					conn = newClientConnection(t.Context(), transport, runner, id, id, &protocol.DefaultConnectionIDGenerator{},
						newStatelessResetter(nil), config, &tls.Config{}, 0, false, false, nil, utils.DefaultLogger, protocol.Version1).Conn
				}
				mode := conn.sentPacketHandler.ECNMode(true)
				if bbr && (mode == protocol.ECT0 || mode == protocol.ECT1) {
					t.Errorf("BBRv3 marks packets %v on an ECN-capable transport", mode)
				}
				if !bbr && mode != protocol.ECT0 {
					t.Errorf("upstream ECN mode = %v, want ECT(0)", mode)
				}
			})
		}
	}
}
