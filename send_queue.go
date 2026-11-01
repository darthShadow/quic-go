package quic

import (
	"net"

	"github.com/quic-go/quic-go/internal/protocol"
)

type sender interface {
	Send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN)
	SendProbe(*packetBuffer, net.Addr, packetInfo)
	Run() error
	WouldBlock() bool
	Available() <-chan struct{}
	Close()
}

type queueEntry struct {
	isMTUProbe bool
	buf        *packetBuffer
	gsoSize    uint16
	ecn        protocol.ECN
}

type sendQueue struct {
	queue         chan queueEntry
	closeCalled   chan struct{} // runStopped when Close() is called
	runStopped    chan struct{} // runStopped when the run loop returns
	available     chan struct{}
	conn          sendConn
	msgSizeErrors chan protocol.ByteCount
}

var _ sender = &sendQueue{}

const sendQueueCapacity = 8

func newSendQueue(conn sendConn, enabled ...bool) sender {
	var notifications chan protocol.ByteCount
	if len(enabled) > 0 && enabled[0] {
		notifications = make(chan protocol.ByteCount, 1)
	}
	return &sendQueue{
		conn:          conn,
		msgSizeErrors: notifications,
		runStopped:    make(chan struct{}),
		closeCalled:   make(chan struct{}),
		available:     make(chan struct{}, 1),
		queue:         make(chan queueEntry, sendQueueCapacity),
	}
}

// Send sends out a packet. It's guaranteed to not block.
// Callers need to make sure that there's actually space in the send queue by calling WouldBlock.
// Otherwise Send will panic.
func (h *sendQueue) Send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN) {
	h.send(p, gsoSize, ecn, false)
}

// SendMTUProbe queues a discovery packet whose send failure is handled as probe loss.
func (h *sendQueue) SendMTUProbe(p *packetBuffer, ecn protocol.ECN) {
	h.send(p, 0, ecn, true)
}

func (h *sendQueue) send(p *packetBuffer, gsoSize uint16, ecn protocol.ECN, isMTUProbe bool) {
	select {
	case h.queue <- queueEntry{buf: p, gsoSize: gsoSize, ecn: ecn, isMTUProbe: isMTUProbe}:
		// clear available channel if we've reached capacity
		if len(h.queue) == sendQueueCapacity {
			select {
			case <-h.available:
			default:
			}
		}
	case <-h.runStopped:
	default:
		panic("sendQueue.Send would have blocked")
	}
}

func (h *sendQueue) SendProbe(p *packetBuffer, addr net.Addr, info packetInfo) {
	h.conn.WriteTo(p.Data, addr, info)
}

func (h *sendQueue) WouldBlock() bool {
	return len(h.queue) == sendQueueCapacity
}

// SendMsgSizeErrors wakes the connection loop without changing send-queue ownership.
func (h *sendQueue) SendMsgSizeErrors() <-chan protocol.ByteCount { return h.msgSizeErrors }

func (h *sendQueue) Available() <-chan struct{} {
	return h.available
}

func (h *sendQueue) Run() error {
	defer close(h.runStopped)
	var shouldClose bool
	for {
		if shouldClose && len(h.queue) == 0 {
			return nil
		}
		select {
		case <-h.closeCalled:
			h.closeCalled = nil // prevent this case from being selected again
			// make sure that all queued packets are actually sent out
			shouldClose = true
		case e := <-h.queue:
			if err := h.conn.Write(e.buf.Data, e.gsoSize, e.ecn); err != nil {
				// This additional check enables:
				// 1. Checking for "datagram too large" message from the kernel, as such,
				// 2. Path MTU discovery,and
				// 3. Eventual detection of loss PingFrame.
				if !isSendMsgSizeErr(err) {
					return err
				}
				size := e.buf.Len()
				if e.gsoSize > 0 {
					size = protocol.ByteCount(e.gsoSize)
				}
				// Probes are already registered with the loss detector. Let their
				// existing mtuFinder handler narrow the search when declared lost.
				if !e.isMTUProbe && size > protocol.MinInitialPacketSize {
					select {
					case h.msgSizeErrors <- size:
					default:
					}
				}
			}
			e.buf.Release()
			select {
			case h.available <- struct{}{}:
			default:
			}
		}
	}
}

func (h *sendQueue) Close() {
	close(h.closeCalled)
	// wait until the run loop returned
	<-h.runStopped
}
