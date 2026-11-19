package quic

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"reflect"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
)

// The boundary keeps real QUIC, packet packing, ACKs, PTOs, and UDP socket I/O.
// Only the path's size/error/loss behavior is controlled here.
type benchMTUPath struct {
	detect bool
	net.PacketConn
	limit         atomic.Int64
	attempts      atomic.Uint64
	dropped       atomic.Uint64
	randomDropped atomic.Uint64
	burstDropped  atomic.Uint64
	emsgsize      atomic.Bool
	randomLoss    atomic.Bool
}

func (*benchMTUPath) SupportsDF() bool { return true }
func (p *benchMTUPath) SyscallConn() (syscall.RawConn, error) {
	return p.PacketConn.(*net.UDPConn).SyscallConn()
}

func (p *benchMTUPath) WriteTo(b []byte, addr net.Addr) (int, error) {
	n := p.attempts.Add(1)
	if limit := p.limit.Load(); limit > 0 && int64(len(b)) > limit {
		p.dropped.Add(1)
		if p.emsgsize.Load() {
			return 0, &net.OpError{Op: "write", Net: "udp", Err: syscall.EMSGSIZE}
		}
		return len(b), nil
	}
	// A seeded deterministic 1% loss schedule plus four consecutive losses.
	if p.randomLoss.Load() {
		if n >= 250 && n < 254 {
			p.burstDropped.Add(1)
			p.dropped.Add(1)
			return len(b), nil
		}
		if (n*6364136223846793005+1442695040888963407)%100 == 0 {
			p.randomDropped.Add(1)
			p.dropped.Add(1)
			return len(b), nil
		}
	}
	return p.PacketConn.WriteTo(b, addr)
}

type benchMTUTrace struct {
	size             atomic.Int64
	grown            atomic.Bool
	pto              atomic.Uint32
	firstFallbackPTO atomic.Uint32
	fallbackSeen     atomic.Bool
	fallbacks        atomic.Uint32
}

func (r *benchMTUTrace) New(context.Context, bool, ConnectionID) qlogwriter.Trace { return r }
func (r *benchMTUTrace) AddProducer() qlogwriter.Recorder                         { return r }
func (*benchMTUTrace) SupportsSchemas(s string) bool                              { return s == qlog.EventSchema }
func (*benchMTUTrace) Close() error                                               { return nil }
func (r *benchMTUTrace) RecordEvent(e qlogwriter.Event) {
	if event, ok := e.(qlog.PTOCountUpdated); ok {
		r.pto.Store(event.PTOCount)
	}
	if m, ok := e.(qlog.MTUUpdated); ok {
		if m.Value == 1200 {
			r.fallbacks.Add(1)
		}
		if m.Value == 1200 && r.fallbackSeen.CompareAndSwap(false, true) {
			r.firstFallbackPTO.Store(r.pto.Load())
		}
		r.size.Store(int64(m.Value))
		if m.Value > 1400 {
			r.grown.Store(true)
		}
	}
}

func benchMTUConnect(t *testing.T, path *benchMTUPath, trace *benchMTUTrace) (*Conn, *Conn) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	config := &Config{MaxIdleTimeout: 2 * time.Second, Tracer: trace.New}
	// A tests-only overlay also runs on upstream before the opt-in field exists.
	if f := reflect.ValueOf(config).Elem().FieldByName("PMTUBlackHoleDetection"); f.IsValid() {
		f.SetBool(path.detect)
	}
	listener, err := Listen(path, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, NextProtos: []string{"bench-mtu"}}, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client, err := DialAddr(ctx, listener.Addr().String(), &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"bench-mtu"}}, &Config{MaxIdleTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.CloseWithError(0, "") })
	server, err := listener.Accept(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.CloseWithError(0, "") })
	return server, client
}

func benchMTUPair(t *testing.T, path *benchMTUPath, trace *benchMTUTrace) (*Conn, *Conn) {
	t.Helper()
	server, client := benchMTUConnect(t, path, trace)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Keep sending until actual probe acknowledgments grow the path above 1400.
	warm, err := server.OpenUniStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() {
		data := make([]byte, 16384)
		for !trace.grown.Load() {
			if _, err := warm.Write(data); err != nil {
				written <- err
				return
			}
		}
		written <- warm.Close()
	}()
	receive, err := client.AcceptUniStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, receive); err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if !trace.grown.Load() {
		t.Fatal("path did not grow above 1400")
	}
	return server, client
}

func TestPMTUBlackHoleFallback(t *testing.T) {
	for _, mode := range []string{"silent-drop", "EMSGSIZE", "random-loss-and-burst", "EMSGSIZE-from-start", "silent-drop-disabled"} {
		t.Run(mode, func(t *testing.T) {
			udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer udp.Close()
			path := &benchMTUPath{PacketConn: udp, detect: mode != "silent-drop-disabled"}
			var trace benchMTUTrace
			before := trace.fallbacks.Load()
			var server, client *Conn
			if mode == "EMSGSIZE-from-start" {
				path.emsgsize.Store(true)
				path.limit.Store(1252)
				server, client = benchMTUConnect(t, path, &trace)
			} else {
				server, client = benchMTUPair(t, path, &trace)
			}
			if mode == "random-loss-and-burst" {
				path.attempts.Store(0)
				path.randomLoss.Store(true)
			} else {
				path.emsgsize.Store(mode == "EMSGSIZE" || mode == "EMSGSIZE-from-start")
				path.limit.Store(1252)
			}
			stream, err := server.OpenUniStream()
			if err != nil {
				t.Fatal(err)
			}
			data := bytes.Repeat([]byte("blackhole-recovery"), 1<<19)
			sent := make(chan error, 1)
			go func() {
				_, err := stream.Write(data)
				if err == nil {
					err = stream.Close()
				}
				sent <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			receive, err := client.AcceptUniStream(ctx)
			if mode == "silent-drop-disabled" && err != nil {
				if trace.fallbacks.Load() != 0 {
					t.Fatal("default config fell back")
				}
				if err := <-sent; err == nil {
					t.Fatal("disabled fallback unexpectedly completed the transfer")
				}
				return
			}
			if err != nil {
				t.Fatalf("transfer resumes after %s: %v", mode, err)
			}
			got, err := io.ReadAll(receive)
			if mode == "silent-drop-disabled" {
				if err == nil {
					t.Fatal("disabled fallback unexpectedly completed the transfer")
				}
				if trace.fallbacks.Load() != 0 {
					t.Fatal("default config fell back")
				}
				<-sent
				return
			}
			if err != nil {
				t.Fatalf("transfer resumes after %s: bytes=%d/%d: %v", mode, len(got), len(data), err)
			}
			if err := <-sent; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("payload length=%d want %d or content differs", len(got), len(data))
			}
			if path.dropped.Load() == 0 {
				t.Fatal("path did not exercise the required loss/error")
			}
			after := trace.fallbacks.Load()
			switch mode {
			case "EMSGSIZE-from-start":
				if before != after {
					t.Fatalf("failed upward probes triggered fallback: before=%v after=%v", before, after)
				}
				if err := server.CloseWithError(0, ""); err != nil {
					t.Fatal(err)
				}
				if size := trace.size.Load(); size < 1200 || size > 1252 || !server.mtuDiscoverer.done() {
					t.Fatalf("discovered MTU=%d done=%t want settled 1200..1252", size, server.mtuDiscoverer.done())
				}
				t.Logf("settled MTU=%d dropped=%d fallbacks=%d", trace.size.Load(), path.dropped.Load(), after-before)
			case "random-loss-and-burst":
				if before != after {
					t.Fatalf("random loss triggered fallback: before=%v after=%v", before, after)
				}
				if path.burstDropped.Load() != 4 || path.randomDropped.Load() == 0 {
					t.Fatalf("burst drops=%d random drops=%d", path.burstDropped.Load(), path.randomDropped.Load())
				}
			default:
				if before == after {
					t.Fatal("missing fallback evidence")
				}
				if mode == "EMSGSIZE" && after-before != 1 {
					t.Fatalf("EMSGSIZE fallbacks=%d want 1", after-before)
				}
				if mode == "silent-drop" && trace.firstFallbackPTO.Load() != 2 {
					t.Fatalf("first fallback at PTO %d want2", trace.firstFallbackPTO.Load())
				}
				if size := trace.size.Load(); size < 1200 || size > 1252 {
					t.Fatalf("recovered MTU=%d want 1200..1252", size)
				}
				t.Logf("recovered MTU=%d dropped=%d fallbacks before=%v after=%v", trace.size.Load(), path.dropped.Load(), before, after)
			}
		})
	}
}

func TestPMTUSocketErrorReachesQUIC(t *testing.T) {
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	path := &benchMTUPath{PacketConn: udp}
	path.emsgsize.Store(true)
	path.limit.Store(1252)
	raw, err := wrapConn(path)
	if err != nil {
		t.Fatal(err)
	}
	conn := newSendConn(raw, udp.LocalAddr(), packetInfo{}, nil)
	if err := conn.Write(make([]byte, 1400), 0, 0); !errors.Is(err, syscall.EMSGSIZE) {
		t.Fatalf("socket EMSGSIZE reaches sendConn.Write: %v", err)
	}
}
