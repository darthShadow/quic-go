package quic

import (
	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/protocol"
	"github.com/quic-go/quic-go/qlog"
)

// Optional capabilities keep the existing sender interfaces and upstream mocks
// unchanged. Real send queues and sent-packet handlers provide these methods.
func (c *Conn) sendMsgSizeErrors() <-chan protocol.ByteCount {
	if !c.config.PMTUBlackHoleDetection {
		return nil
	}
	if q, ok := c.sendQueue.(interface {
		SendMsgSizeErrors() <-chan protocol.ByteCount
	}); ok {
		return q.SendMsgSizeErrors()
	}
	return nil
}

func (c *Conn) pmtuPTOCount() uint32 {
	if !c.config.PMTUBlackHoleDetection {
		return 0
	}
	if h, ok := c.sentPacketHandler.(interface{ PTOCount() uint32 }); ok {
		return h.PTOCount()
	}
	return 0
}

func (c *Conn) sendMTUProbe(buf *packetBuffer, ecn protocol.ECN) {
	if c.config.PMTUBlackHoleDetection {
		if q, ok := c.sendQueue.(interface {
			SendMTUProbe(*packetBuffer, protocol.ECN)
		}); ok {
			q.SendMTUProbe(buf, ecn)
			return
		}
	}
	c.sendQueue.Send(buf, 0, ecn)
}

func (c *Conn) handleDataMsgSizeError(now monotime.Time, size protocol.ByteCount) {
	if c.mtuDiscoverer != nil && size == c.mtuDiscoverer.CurrentSize() {
		c.recoverPathMTU(now, "EMSGSIZE")
	}
}

// Recovery runs on the connection loop and retains congestion and loss state.
func (c *Conn) recoverPathMTU(now monotime.Time, reason string) {
	if !c.config.PMTUBlackHoleDetection || !c.handshakeConfirmed || c.mtuDiscoverer == nil || c.mtuDiscoverer.CurrentSize() <= protocol.MinInitialPacketSize {
		return
	}
	before := c.mtuDiscoverer.CurrentSize()
	c.mtuDiscoverer.Reset(protocol.MinInitialPacketSize, c.maxMTU())
	c.maxPayloadSizeEstimate.Store(uint32(estimateMaxPayloadSize(protocol.MinInitialPacketSize)))
	c.sentPacketHandler.SetMaxDatagramSize(protocol.MinInitialPacketSize)
	if !c.config.DisablePathMTUDiscovery && c.conn.capabilities().DF {
		c.mtuDiscoverer.Start(now)
	}
	recordMTUFallback(int(before), reason)
	if c.qlogger != nil {
		c.qlogger.RecordEvent(qlog.MTUUpdated{Value: int(protocol.MinInitialPacketSize), Done: false})
	}
}
