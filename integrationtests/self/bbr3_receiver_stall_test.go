package self_test

import (
	"context"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
	"github.com/quic-go/quic-go/testutils/simnet"
)

type stallItem struct {
	p  simnet.Packet
	at time.Time
}

// stallShaper models one netem qdisc: random loss at enqueue, then a packet
// limit covering both the rate queue and the delay line, then rate and delay.
type stallShaper struct {
	rate   float64 // bytes per second
	delay  time.Duration
	limit  int
	loss   float64
	rng    *rand.Rand
	router *stallRouter

	mu         sync.Mutex
	cond       *sync.Cond
	q          []stallItem
	lastDepart time.Time
	closed     bool
}

func (s *stallShaper) enqueue(p simnet.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loss > 0 && s.rng.Float64() < s.loss {
		return
	}
	if len(s.q) >= s.limit {
		return
	}
	now := time.Now()
	depart := now
	if s.lastDepart.After(depart) {
		depart = s.lastDepart
	}
	depart = depart.Add(time.Duration(float64(len(p.Data)+28) / s.rate * float64(time.Second)))
	s.lastDepart = depart
	s.q = append(s.q, stallItem{p: p, at: depart.Add(s.delay)})
	s.cond.Signal()
}

func (s *stallShaper) run() {
	var batch []stallItem
	for {
		s.mu.Lock()
		for len(s.q) == 0 && !s.closed {
			s.cond.Wait()
		}
		if s.closed {
			s.mu.Unlock()
			return
		}
		now := time.Now()
		wait := s.q[0].at.Sub(now)
		if wait > 0 {
			s.mu.Unlock()
			time.Sleep(wait)
			continue
		}
		n := 0
		for n < len(s.q) && !s.q[n].at.After(now) {
			n++
		}
		batch = append(batch[:0], s.q[:n]...)
		s.q = append(s.q[:0], s.q[n:]...)
		s.mu.Unlock()
		for _, it := range batch {
			if dst, ok := s.router.node(it.p.To); ok {
				dst.RecvPacket(it.p)
			}
		}
	}
}

type stallRouter struct {
	mu     sync.Mutex
	nodes  map[string]simnet.PacketReceiver
	shaper map[string]*stallShaper // keyed by source address
}

func (r *stallRouter) node(a net.Addr) (simnet.PacketReceiver, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.nodes[a.String()]
	return n, ok
}

func (r *stallRouter) SendPacket(p simnet.Packet) error {
	r.mu.Lock()
	s := r.shaper[p.From.String()]
	r.mu.Unlock()
	s.enqueue(p)
	return nil
}

func (r *stallRouter) AddNode(addr net.Addr, rcv simnet.PacketReceiver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[addr.String()] = rcv
}

// stallConn reports DF so path MTU discovery runs, and optionally GSO.
type stallConn struct {
	*simnet.SimConn
	gso bool
}

func (c *stallConn) SupportsDF() bool  { return true }
func (c *stallConn) SupportsGSO() bool { return c.gso }
func (c *stallConn) WriteSegments(b []byte, addr net.Addr, size uint16) (int, error) {
	if size == 0 {
		return c.WriteTo(b, addr)
	}
	for off := 0; off < len(b); off += int(size) {
		end := min(off+int(size), len(b))
		if _, err := c.WriteTo(b[off:end], addr); err != nil {
			return off, err
		}
	}
	return len(b), nil
}

// stallObserver measures the sender's stream block at the packet boundary.
type stallObserver struct {
	mu         sync.Mutex
	blockedAt  time.Time
	limit      int64
	maxBlocked time.Duration
}

func (o *stallObserver) AddProducer() qlogwriter.Recorder { return o }
func (o *stallObserver) SupportsSchemas(string) bool      { return true }
func (o *stallObserver) Close() error                     { return nil }
func (o *stallObserver) RecordEvent(event qlogwriter.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	switch e := event.(type) {
	case qlog.PacketSent:
		for _, f := range e.Frames {
			if b, ok := f.Frame.(*qlog.StreamDataBlockedFrame); ok && o.blockedAt.IsZero() {
				o.blockedAt, o.limit = time.Now(), int64(b.MaximumStreamData)
			}
		}
	case qlog.PacketReceived:
		for _, f := range e.Frames {
			if u, ok := f.Frame.(*qlog.MaxStreamDataFrame); ok && !o.blockedAt.IsZero() && int64(u.MaximumStreamData) > o.limit {
				o.maxBlocked = max(o.maxBlocked, time.Since(o.blockedAt))
				o.blockedAt = time.Time{}
			}
		}
	}
}

type stallSample struct {
	at    time.Time
	bytes int64
}

type stallGoodputWindow struct {
	at      time.Time
	elapsed time.Duration
	mbps    float64
}

func stallGoodputWindows(samples []stallSample) []stallGoodputWindow {
	start := len(samples)
	for i, sample := range samples {
		if sample.bytes > 48<<20 {
			start = i
			break
		}
	}
	var windows []stallGoodputWindow
	for i := start; i+4 < len(samples); i++ {
		// Four 250 ms intervals form each rolling one-second window.
		// Use elapsed wall time to account for timer jitter.
		elapsed := samples[i+4].at.Sub(samples[i].at)
		windows = append(windows, stallGoodputWindow{samples[i].at, elapsed, float64(samples[i+4].bytes-samples[i].bytes) * 8 / 1e6 / elapsed.Seconds()})
	}
	return windows
}

func TestBBR3StallGoodputWindows(t *testing.T) {
	const mib = 1 << 20
	for _, tc := range []struct {
		name          string
		bytes         []int64
		windows, slow int
	}{
		{"empty", nil, 0, 0},
		{"window never exceeded", []int64{44 * mib, 45 * mib, 46 * mib, 47 * mib, 48 * mib, 48 * mib}, 0, 0},
		{"no complete window", []int64{48 * mib, 49 * mib, 50 * mib, 51 * mib, 52 * mib}, 0, 0},
		{"one window", []int64{48 * mib, 49 * mib, 50 * mib, 51 * mib, 52 * mib, 53 * mib}, 1, 0},
		{"two windows", []int64{48 * mib, 49 * mib, 50 * mib, 51 * mib, 52 * mib, 53 * mib, 54 * mib}, 2, 0},
		{"late burst preserves early stall", []int64{48 * mib, 49 * mib, 49 * mib, 49 * mib, 49 * mib, 49 * mib, 49 * mib, 70 * mib}, 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var samples []stallSample
			for i, n := range tc.bytes {
				samples = append(samples, stallSample{time.Unix(0, 0).Add(time.Duration(i) * 250 * time.Millisecond), n})
			}
			windows := stallGoodputWindows(samples)
			slow := 0
			for _, window := range windows {
				if window.mbps < 1 {
					slow++
				}
			}
			if len(windows) != tc.windows || slow != tc.slow {
				t.Fatalf("windows=%d, slow=%d; want windows=%d, slow=%d", len(windows), slow, tc.windows, tc.slow)
			}
		})
	}
}

// This regression covers the receiver signal. The pacer rule is covered by
// TestBBR3PacerChargesOnlyAdmittedPackets in internal/congestion.
func TestBBR3ReceiverDoesNotStallFlowControl(t *testing.T) {
	t.Setenv("QG_BBR_DEFAULTS", "c2")
	router := &stallRouter{nodes: map[string]simnet.PacketReceiver{}, shaper: map[string]*stallShaper{}}
	clientAddr := &net.UDPAddr{IP: net.ParseIP("1.0.0.1"), Port: 9001}
	serverAddr := &net.UDPAddr{IP: net.ParseIP("1.0.0.2"), Port: 9002}
	var workers sync.WaitGroup
	for i, addr := range []net.Addr{serverAddr, clientAddr} {
		loss := 0.001
		if i == 1 {
			loss = 0
		}
		seed := uint64(i + 1)
		s := &stallShaper{rate: 25_000_000, delay: 175 * time.Millisecond, limit: 5834, loss: loss, rng: rand.New(rand.NewPCG(seed, seed^0x9e37)), router: router}
		s.cond = sync.NewCond(&s.mu)
		router.shaper[addr.String()] = s
		workers.Go(s.run)
	}
	defer func() {
		for _, s := range router.shaper {
			s.mu.Lock()
			s.closed = true
			s.cond.Broadcast()
			s.mu.Unlock()
		}
		workers.Wait()
	}()
	policy := func() *quic.Config {
		return &quic.Config{
			InitialPacketSize: 1200, HandshakeIdleTimeout: 20 * time.Second, MaxIdleTimeout: 10 * time.Second, KeepAlivePeriod: 3 * time.Second,
			CongestionControl: quic.CongestionControlBBRv3, BBRNoLowerBounds: true, BBRLossThresholdPercent: 5,
			PMTUBlackHoleDetection: true, ScaledStreamGapLimit: true,
			InitialStreamReceiveWindow: 48 << 20, MaxStreamReceiveWindow: 48 << 20,
			InitialConnectionReceiveWindow: 64 << 20, MaxConnectionReceiveWindow: 64 << 20,
		}
	}
	observer := &stallObserver{}
	sc, cc := policy(), policy()
	sc.Tracer = func(context.Context, bool, quic.ConnectionID) qlogwriter.Trace { return observer }
	cc.ACKDecimation = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server := &quic.Transport{Conn: &stallConn{SimConn: simnet.NewBlockingSimConn(serverAddr, router), gso: true}}
	defer server.Conn.Close()
	defer server.Close()
	listener, err := server.Listen(getTLSConfig(), sc)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(ctx)
		if err != nil {
			serverErr <- err
			return
		}
		stream, err := conn.OpenStream()
		if err != nil {
			serverErr <- err
			return
		}
		buf := make([]byte, 1<<20)
		for {
			if _, err := stream.Write(buf); err != nil {
				serverErr <- err
				return
			}
		}
	}()
	client := &quic.Transport{Conn: &stallConn{SimConn: simnet.NewBlockingSimConn(clientAddr, router)}}
	defer client.Conn.Close()
	defer client.Close()
	conn, err := client.Dial(ctx, listener.Addr(), getTLSClientConfig(), cc)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseWithError(0, "test complete")
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var received atomic.Int64
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 1<<20)
		for {
			n, err := stream.Read(buf)
			received.Add(int64(n))
			if err != nil {
				readErr <- err
				return
			}
		}
	}()
	samples := []stallSample{{time.Now(), received.Load()}}
	peak := 0.0
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for i := 0; i < 48; i++ {
		select {
		case <-ticker.C:
			s := stallSample{time.Now(), received.Load()}
			prev := samples[len(samples)-1]
			mbps := float64(s.bytes-prev.bytes) * 8 / 1e6 / s.at.Sub(prev.at).Seconds()
			samples = append(samples, s)
			if mbps > peak {
				peak = mbps
			}
		case err := <-readErr:
			t.Fatalf("reader stopped: %v", err)
		case err := <-serverErr:
			t.Fatalf("sender stopped: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if peak < 50 || received.Load() <= 48<<20 {
		t.Fatalf("path did not exercise the window: peak=%.1f Mbit/s, read=%d", peak, received.Load())
	}
	windows := stallGoodputWindows(samples)
	for _, window := range windows {
		if window.mbps < 1 {
			t.Errorf("post-window-limit window at %.2fs: %.3f Mbit/s over %v", window.at.Sub(samples[0].at).Seconds(), window.mbps, window.elapsed)
		}
	}
	if len(windows) == 0 {
		t.Error("no complete one-second window after exceeding the stream window")
	}
	observer.mu.Lock()
	blocked := observer.maxBlocked
	if !observer.blockedAt.IsZero() {
		blocked = max(blocked, time.Since(observer.blockedAt))
	}
	observer.mu.Unlock()
	t.Logf("peak=%.1f Mbit/s, read=%d, post-window-limit windows=%d, max stream block=%v", peak, received.Load(), len(windows), blocked)
	if blocked > time.Second {
		t.Errorf("sender stream-flow-control block=%v, want <=1s", blocked)
	}
}
