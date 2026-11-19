package quic

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"slices"
	"testing"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/utils"
	"github.com/quic-go/quic-go/internal/wire"
)

func TestScaledGapMatchesUpstream(t *testing.T) {
	for seed := uint64(0); seed < 400; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, 0))
			scaled := newReceiveStreamWithWindow(48<<20, true).frameQueue
			upstream := newFrameSorter()
			upstream.gapLimit = (48<<20)/1024 + 1
			for step := 0; step < 600; step++ {
				offset := protocol.ByteCount(rng.IntN(4096))
				data := make([]byte, rng.IntN(64))
				gotErr := scaled.Push(data, offset, nil)
				wantErr := upstream.Push(data, offset, nil)
				if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
					t.Fatalf("step %d offset %d len %d: error = %v, want %v", step, offset, len(data), gotErr, wantErr)
				}
				gotGap, wantGap := scaled.gaps.Front(), upstream.gaps.Front()
				for gotGap != nil && wantGap != nil {
					if gotGap.Value != wantGap.Value {
						t.Fatalf("step %d offset %d len %d: gap = %v, want %v", step, offset, len(data), gotGap.Value, wantGap.Value)
					}
					gotGap, wantGap = gotGap.Next(), wantGap.Next()
				}
				if gotGap != nil || wantGap != nil {
					t.Fatalf("step %d: gap counts = %d, want %d", step, scaled.gaps.Len(), upstream.gaps.Len())
				}
				if len(scaled.queue) != len(upstream.queue) {
					t.Fatalf("step %d: queue lengths = %d, want %d", step, len(scaled.queue), len(upstream.queue))
				}
				for key := range scaled.queue {
					if _, ok := upstream.queue[key]; !ok {
						t.Fatalf("step %d: unexpected queue key %d", step, key)
					}
				}
				if rng.IntN(4) == 0 {
					gotOffset, gotData, _ := scaled.Pop()
					wantOffset, wantData, _ := upstream.Pop()
					if gotOffset != wantOffset || !slices.Equal(gotData, wantData) {
						t.Fatalf("step %d: pop = (%d, %v), want (%d, %v)", step, gotOffset, gotData, wantOffset, wantData)
					}
				}
			}
		})
	}
}

func TestScaledGapFrontSearchBound(t *testing.T) {
	s := newReceiveStreamWithWindow(48<<20, true).frameQueue
	s.gaps.Front().Value.End = 2
	for i := 1; i < 49152; i++ {
		s.gaps.PushBack(byteInterval{Start: protocol.ByteCount(i * 4), End: protocol.ByteCount(i*4 + 2)})
	}
	s.gaps.PushBack(byteInterval{Start: 49152 * 4, End: protocol.MaxByteCount})
	// Compare identical 1-byte front-gap searches. A benchmark adds no hook or
	// counter to either production path; the allowance absorbs timing noise.
	measure := func(scaled bool) testing.BenchmarkResult {
		fc := s.flowController
		if !scaled {
			s.flowController = nil
		}
		defer func() { s.flowController = fc }()
		return testing.Benchmark(func(b *testing.B) {
			for b.Loop() {
				start, _ := s.findStartGap(0)
				end, _ := s.findEndGap(start, 1)
				runtime.KeepAlive(end)
			}
		})
	}
	upstream, scaled := measure(false), measure(true)
	if scaled.NsPerOp() > 10*max(upstream.NsPerOp(), 1) {
		t.Fatalf("front-gap search with %d gaps: scaled %d ns/op exceeds 10 x upstream %d ns/op", s.gaps.Len(), scaled.NsPerOp(), upstream.NsPerOp())
	}
}

func newReceiveStreamWithWindow(window protocol.ByteCount, scaled bool) *ReceiveStream {
	rtt := utils.NewRTTStats()
	cfc := newConnectionFlowController(64<<20, 64<<20, nil, rtt, utils.DefaultLogger)
	c := Conn{config: &Config{InitialStreamReceiveWindow: uint64(window), MaxStreamReceiveWindow: 48 << 20, ScaledStreamGapLimit: scaled}, peerParams: &wire.TransportParameters{}, connFlowController: cfc, rttStats: rtt, logger: utils.DefaultLogger}
	return newReceiveStream(3, nil, c.newFlowController(3))
}

func TestScaledGapLimit(t *testing.T) {
	t.Run("48-MiB-scaled", func(t *testing.T) {
		s := newReceiveStreamWithWindow(48<<20, true)
		for i := 0; i < 2000; i++ {
			if err := s.handleStreamFrame(&wire.StreamFrame{Offset: protocol.ByteCount(i*4800 + 2400), Data: make([]byte, 2400)}, monotime.Now()); err != nil {
				t.Fatalf("burst %d: %v", i, err)
			}
		}
		if got := s.frameQueue.gaps.Len(); got != 2001 {
			t.Fatalf("gaps=%d, want 2001", got)
		}
	})
	t.Run("default-1000", func(t *testing.T) {
		s := newReceiveStreamWithWindow(48<<20, false)
		for i := 1; i < 1000; i++ {
			if err := s.handleStreamFrame(&wire.StreamFrame{Offset: protocol.ByteCount(2*i - 1), Data: []byte{1}}, monotime.Now()); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.handleStreamFrame(&wire.StreamFrame{Offset: 1999, Data: []byte{1}}, monotime.Now()); err == nil || err.Error() != "too many gaps in received data" {
			t.Fatalf("1001 gaps: error=%v", err)
		}
	})
	t.Run("current-window-growth", func(t *testing.T) {
		s := newReceiveStreamWithWindow(1<<20, true)
		for i := 1; i <= 1024; i++ {
			if err := s.handleStreamFrame(&wire.StreamFrame{Offset: protocol.ByteCount(2*i - 1), Data: []byte{1}}, monotime.Now()); err != nil {
				t.Fatalf("gap %d: %v", i+1, err)
			}
		}
		s.flowController.mutex.Lock()
		s.flowController.receiveWindowSize = 2 << 20
		s.flowController.mutex.Unlock()
		for i := 1025; i <= 2048; i++ {
			if err := s.handleStreamFrame(&wire.StreamFrame{Offset: protocol.ByteCount(2*i - 1), Data: []byte{1}}, monotime.Now()); err != nil {
				t.Fatalf("gap %d: %v", i+1, err)
			}
		}
		if err := s.handleStreamFrame(&wire.StreamFrame{Offset: 4097, Data: []byte{1}}, monotime.Now()); err == nil || err.Error() != "too many gaps in received data" {
			t.Fatalf("2050 gaps: error=%v", err)
		}
	})
	t.Run("crypto-fixed", func(t *testing.T) {
		s := newCryptoStream()
		for i := 1; i < 1000; i++ {
			if err := s.HandleCryptoFrame(&wire.CryptoFrame{Offset: protocol.ByteCount(2*i - 1), Data: []byte{1}}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.HandleCryptoFrame(&wire.CryptoFrame{Offset: 1999, Data: []byte{1}}); err == nil || err.Error() != "too many gaps in received data" {
			t.Fatalf("CRYPTO 1001 gaps: error=%v", err)
		}
	})
}
