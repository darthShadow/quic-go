package quic

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/quic-go/quic-go/internal/ackhandler"
	mockackhandler "github.com/quic-go/quic-go/internal/mocks/ackhandler"
	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/internal/wire"

	"go.uber.org/mock/gomock"
)

func TestConnectionAppLimitedSignal(t *testing.T) {
	for _, gso := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			for _, state := range []string{"idle", "queued", "writing", "write-data", "retransmission", "stream-blocked", "connection-blocked", "outgoing-bidi", "incoming-bidi"} {
				for _, mode := range []ackhandler.SendMode{ackhandler.SendAny, ackhandler.SendAck, ackhandler.SendPacingLimited, ackhandler.SendNone} {
					t.Run(fmt.Sprintf("%s/mode=%d/gso=%t/confirmed=%t", state, mode, gso, confirmed), func(t *testing.T) {
						ctrl := gomock.NewController(t)
						handler := mockackhandler.NewMockSentPacketHandler(ctrl)
						tc := newServerTestConnection(t, ctrl, nil, gso, connectionOptSentPacketHandler(handler))
						tc.conn.handshakeConfirmed = confirmed
						tc.conn.connFlowController.UpdateSendWindow(1 << 20)
						fc := newStreamFlowController(3, tc.conn.connFlowController, 0, 0, 1<<20, tc.conn.rttStats, tc.conn.logger)
						str := newSendStream(tc.conn.ctx, 3, tc.conn, fc, false)
						tc.conn.streamsMap.outgoingUniStreams.streams[3] = str
						switch state {
						case "queued":
							str.nextFrame = &wire.StreamFrame{Data: []byte("pending")}
						case "writing":
							// A Write can still be waking up after its last frame is packed.
							str.writeOnce <- struct{}{}
						case "write-data":
							str.dataForWriting = []byte("pending")
						case "retransmission":
							str.retransmissionQueue = []*wire.StreamFrame{{Data: []byte("lost")}}
						case "outgoing-bidi":
							tc.conn.streamsMap.outgoingBidiStreams.streams[1] = &Stream{sendStr: newSendStream(tc.conn.ctx, 1, tc.conn, fc, false)}
							tc.conn.streamsMap.outgoingBidiStreams.streams[1].sendStr.dataForWriting = []byte("pending")
						case "incoming-bidi":
							tc.conn.streamsMap.incomingBidiStreams.streams[0] = incomingStreamEntry[*Stream]{stream: &Stream{sendStr: newSendStream(tc.conn.ctx, 0, tc.conn, fc, false)}}
							tc.conn.streamsMap.incomingBidiStreams.streams[0].stream.sendStr.dataForWriting = []byte("pending")
						case "stream-blocked":
							fc.bytesSent = fc.sendWindow
						case "connection-blocked":
							tc.conn.connFlowController.TryAddBytesSent(1 << 20)
						}
						handler.EXPECT().SendMode(gomock.Any()).Return(mode).Times(2)
						count := 0
						if mode == ackhandler.SendAny && state == "idle" {
							count = 1
						}
						handler.EXPECT().OnAppLimited().Times(0)
						if mode != ackhandler.SendNone {
							if confirmed {
								handler.EXPECT().ECNMode(true).AnyTimes()
								if mode == ackhandler.SendAny {
									tc.packer.EXPECT().AppendPacket(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(shortHeaderPacket{}, errNothingToPack).Times(2)
								} else {
									tc.packer.EXPECT().PackAckOnlyPacket(gomock.Any(), gomock.Any(), gomock.Any()).Return(shortHeaderPacket{}, nil, errNothingToPack).Times(2)
								}
							} else {
								if mode != ackhandler.SendAny {
									handler.EXPECT().ECNMode(false).Times(2)
								}
								tc.packer.EXPECT().PackCoalescedPacket(mode != ackhandler.SendAny, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(2)
							}
						}
						if mode == ackhandler.SendPacingLimited {
							handler.EXPECT().TimeUntilSend().Return(monotime.Now().Add(time.Second)).Times(2)
						}
						if err := tc.conn.triggerSending(monotime.Now()); err != nil {
							t.Fatal(err)
						}
						handler.EXPECT().OnAppLimited().Times(count)
						if err := tc.conn.triggerSending(monotime.Now()); err != nil {
							t.Fatal(err)
						}
					})
				}
			}
		}
	}
}

func TestConnectionAppLimitedAfterWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		handler := mockackhandler.NewMockSentPacketHandler(ctrl)
		tc := newServerTestConnection(t, ctrl, nil, false, connectionOptSentPacketHandler(handler))
		tc.conn.handshakeConfirmed = true
		tc.conn.connFlowController.UpdateSendWindow(1 << 20)
		fc := newStreamFlowController(3, tc.conn.connFlowController, 0, 0, 1<<20, tc.conn.rttStats, tc.conn.logger)
		str := newSendStream(tc.conn.ctx, 3, tc.conn, fc, false)
		defer str.closeForShutdown(ErrStreamClosed)
		tc.conn.streamsMap.outgoingUniStreams.streams[3] = str
		written := make(chan error, 1)
		go func() {
			_, err := str.Write(make([]byte, 32<<10))
			written <- err
		}()
		synctest.Wait()
		handler.EXPECT().SendMode(gomock.Any()).Return(ackhandler.SendAny).Times(3)
		handler.EXPECT().ECNMode(true).Times(3)
		tc.packer.EXPECT().AppendPacket(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(shortHeaderPacket{}, errNothingToPack).Times(3)
		handler.EXPECT().OnAppLimited().Times(0)
		if err := tc.conn.triggerSending(monotime.Now()); err != nil {
			t.Fatal(err)
		}
		// Let the real Write finish, including its final buffered frame.
		for {
			frame, _, more := str.popStreamFrame(1200, protocol.Version1)
			if frame.Frame != nil {
				frame.Frame.PutBack()
			}
			if !more {
				break
			}
		}
		if err := <-written; err != nil {
			t.Fatal(err)
		}
		frame, _, _ := str.popStreamFrame(1200, protocol.Version1)
		if frame.Frame != nil {
			frame.Frame.PutBack()
		}
		if err := tc.conn.triggerSending(monotime.Now()); err != nil {
			t.Fatal(err)
		}
		handler.EXPECT().OnAppLimited().Times(1)
		if err := tc.conn.triggerSending(monotime.Now()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestConnectionAppLimitedInterveningWrite(t *testing.T) {
	for _, kind := range []string{"receiver", "flow-control-reset", "outgoing-uni", "outgoing-bidi", "incoming-bidi"} {
		for _, tryWrite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/try=%t", kind, tryWrite), func(t *testing.T) {
				ctrl := gomock.NewController(t)
				handler := mockackhandler.NewMockSentPacketHandler(ctrl)
				tc := newServerTestConnection(t, ctrl, nil, false, connectionOptSentPacketHandler(handler))
				tc.conn.handshakeConfirmed = true
				tc.conn.connFlowController.UpdateSendWindow(1 << 20)
				tc.conn.peerParams = &wire.TransportParameters{}
				m := tc.conn.streamsMap
				m.HandleMaxStreamsFrame(&wire.MaxStreamsFrame{Type: protocol.StreamTypeUni, MaxStreamNum: 10})
				m.HandleMaxStreamsFrame(&wire.MaxStreamsFrame{Type: protocol.StreamTypeBidi, MaxStreamNum: 10})
				var str *SendStream
				var err error
				switch kind {
				case "outgoing-uni":
					str, err = m.OpenUniStream()
				case "outgoing-bidi", "incoming-bidi":
					var bidi *Stream
					if kind == "outgoing-bidi" {
						bidi, err = m.OpenStream()
					} else {
						bidi, err = m.incomingBidiStreams.GetOrOpenStream(0)
					}
					if err == nil {
						str = bidi.sendStr
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if str != nil {
					str.updateSendWindow(1 << 20)
				}
				emptyPass := func(count int) {
					t.Helper()
					handler.EXPECT().SendMode(gomock.Any()).Return(ackhandler.SendAny).Times(1)
					handler.EXPECT().ECNMode(true).Times(1)
					tc.packer.EXPECT().AppendPacket(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(shortHeaderPacket{}, errNothingToPack).Times(1)
					handler.EXPECT().OnAppLimited().Times(count)
					if err := tc.conn.triggerSending(monotime.Now()); err != nil {
						t.Fatal(err)
					}
				}
				emptyPass(0)
				if kind == "flow-control-reset" {
					if !tc.conn.connFlowController.TryAddBytesSent(1 << 20) {
						t.Fatal("failed to exhaust connection credit")
					}
					emptyPass(0)
					tc.conn.connFlowController.UpdateSendWindow(2 << 20)
					emptyPass(0)
				}
				if str != nil {
					if tryWrite {
						err = str.TryWriteAll([]byte("chunk"))
					} else {
						_, err = str.Write([]byte("chunk"))
					}
					if err != nil {
						t.Fatal(err)
					}
					frame, _, more := str.popStreamFrame(1200, protocol.Version1)
					if frame.Frame == nil || more {
						t.Fatal("chunk did not drain in one frame")
					}
					frame.Frame.PutBack()
					emptyPass(0)
				}
				emptyPass(1)
			})
		}
	}
}
