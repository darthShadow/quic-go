package quic

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
)

func TestProbeNowAfterBackoff(t *testing.T) {
	t.Run("probes delivered", func(t *testing.T) {
		udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer udp.Close()
		path := &benchMTUPath{PacketConn: udp, detect: true}
		var mu sync.Mutex
		var pto uint32
		var deadline time.Time
		var droppedAtPTO2 uint64
		var sentAtPTO2 int
		var capture bool
		var packets [][]qlog.Frame
		trace := &benchMTUTrace{observe: func(e qlogwriter.Event) {
			mu.Lock()
			defer mu.Unlock()
			switch event := e.(type) {
			case qlog.PTOCountUpdated:
				pto = event.PTOCount
				if pto == 2 {
					droppedAtPTO2 = path.dropped.Load()
					sentAtPTO2 = 0
				}
			case qlog.LossTimerUpdated:
				if event.Type == qlog.LossTimerUpdateTypeSet && event.TimerType == qlog.TimerTypePTO {
					deadline = event.Time
				}
			case qlog.PacketSent:
				if pto == 2 {
					sentAtPTO2++
				}
				if capture {
					packets = append(packets, append([]qlog.Frame(nil), event.Frames...))
				}
			}
		}}
		// Discovery PINGs would independently restore progress and invalidate the
		// no-op control. Black-hole detection remains enabled on this path.
		server, client := benchMTUConnectWithConfig(t, path, &Config{
			MaxIdleTimeout:          2 * time.Second,
			Tracer:                  trace.New,
			DisablePathMTUDiscovery: true,
			InitialPacketSize:       1400,
		})
		path.dropAll.Store(true)
		stream, err := server.OpenUniStream()
		if err != nil {
			t.Fatal(err)
		}
		written := make(chan error, 1)
		go func() {
			_, err := stream.Write(make([]byte, 64*1024))
			if err == nil {
				err = stream.Close()
			}
			written <- err
		}()
		wait := time.NewTimer(5 * time.Second)
		defer wait.Stop()
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		var scheduled time.Time
		for {
			mu.Lock()
			ready := pto == 2 && sentAtPTO2 >= 2 && path.dropped.Load() >= droppedAtPTO2+2
			scheduled = deadline
			mu.Unlock()
			if ready {
				break
			}
			select {
			case <-tick.C:
			case <-wait.C:
				t.Fatal("sender did not reach PTOCount 2 with its timeout probes dropped")
			}
		}
		mu.Lock()
		capture = true
		mu.Unlock()
		if !scheduled.After(time.Now()) {
			t.Fatalf("scheduled timeout is already past: %s", scheduled)
		}
		fallbacks := trace.fallbacks.Load()
		path.dropAll.Store(false)
		server.ProbeNow()
		ctx, cancel := context.WithDeadline(context.Background(), scheduled)
		defer cancel()
		receive, err := client.AcceptUniStream(ctx)
		if err != nil {
			t.Fatalf("no stream progress before the scheduled timeout: %v", err)
		}
		if err := receive.SetReadDeadline(scheduled); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(receive, make([]byte, 1)); err != nil {
			t.Fatalf("no stream byte before the scheduled timeout: %v", err)
		}
		if !time.Now().Before(scheduled) {
			t.Fatal("stream progress occurred after the scheduled timeout")
		}
		mu.Lock()
		probes := append([][]qlog.Frame(nil), packets...)
		mu.Unlock()
		if len(probes) < 2 {
			t.Fatalf("got %d packet_sent events, want at least two probes", len(probes))
		}
		for i, frames := range probes[:2] {
			if len(frames) != 1 {
				t.Fatalf("probe %d contains %d frames, want one PING", i, len(frames))
			}
			if _, ok := frames[0].Frame.(*qlog.PingFrame); !ok {
				t.Fatalf("probe %d contains %T, want PING only", i, frames[0].Frame)
			}
		}
		if got := trace.fallbacks.Load(); got != fallbacks {
			t.Fatalf("ProbeNow caused MTU fallback: before %d, after %d", fallbacks, got)
		}
		if _, err := io.Copy(io.Discard, receive); err != nil {
			t.Fatal(err)
		}
		if err := <-written; err != nil {
			t.Fatal(err)
		}
	})
}
