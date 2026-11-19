package quic

import (
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/protocol"
)

type capabilityPacketConn struct {
	gso         bool
	df          bool
	segmented   bool
	segmentSize uint16
}

func (p *capabilityPacketConn) SupportsDF() bool { return p.df }
func (p *capabilityPacketConn) WriteSegments(b []byte, _ net.Addr, size uint16) (int, error) {
	p.segmented = true
	p.segmentSize = size
	return len(b), nil
}

func TestAdapterCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name    string
		df, gso bool
	}{
		{name: "plain"},
		{name: "DF", df: true},
		{name: "GSO", gso: true},
		{name: "DF-and-GSO", df: true, gso: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &capabilityPacketConn{df: tc.df, gso: tc.gso}
			c, err := wrapConn(p)
			if err != nil {
				t.Fatal(err)
			}
			want := connCapabilities{DF: tc.df, GSO: tc.gso, ECN: false}
			if got := c.capabilities(); got != want {
				t.Fatalf("capabilities = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAdapterGSOSegments(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		p := &capabilityPacketConn{gso: enabled}
		c, err := wrapConn(p)
		if err != nil {
			t.Fatal(err)
		}
		size := uint16(0)
		if enabled {
			size = 4
		}
		n, err := c.WritePacket([]byte("abcdefgh"), &net.UDPAddr{}, nil, size, protocol.ECNUnsupported)
		if err != nil {
			t.Fatal(err)
		}
		if n != 8 || p.segmented != enabled || p.segmentSize != size {
			t.Fatalf("GSO=%t: bytes=%d segmented=%t size=%d, want 8 %t %d", enabled, n, p.segmented, p.segmentSize, enabled, size)
		}
	}
}

func (p *capabilityPacketConn) SupportsGSO() bool { return p.gso }
func (*capabilityPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	b[0] = 42
	return 1, &net.UDPAddr{}, nil
}
func (*capabilityPacketConn) WriteTo(b []byte, _ net.Addr) (int, error) { return len(b), nil }
func (*capabilityPacketConn) Close() error                              { return nil }
func (*capabilityPacketConn) LocalAddr() net.Addr                       { return &net.IPAddr{} }
func (*capabilityPacketConn) SetDeadline(time.Time) error               { return nil }
func (*capabilityPacketConn) SetReadDeadline(time.Time) error           { return nil }
func (*capabilityPacketConn) SetWriteDeadline(time.Time) error          { return nil }
