//go:build !shunpike_research

package congestion

import (
	"fmt"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/utils"
)

// Anchor values use the port's Q8 units for gains and microseconds for time.
// The Google columns refer to quiche 7980e3d5d89a; Bbr2Sender is the anchor.
// Loss-policy pairs show default BBR followed by the explicit no-lower, 5% profile.
// ProbeRTT pairs show the interval and whether expiry enters from Startup.
func TestAnchorDefaults(t *testing.T) {
	newSender := func() *bbr3Sender {
		return NewSender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, nil, SenderConfig{BBRv3: true}).(*bbr3Sender)
	}
	b := newSender()
	lossProfile := NewSender(DefaultClock{}, utils.NewRTTStats(), &utils.ConnectionStats{}, 1200, nil, SenderConfig{BBRv3: true, NoLowerBounds: true, LossThresholdPercent: 5}).(*bbr3Sender)
	probeRTT := newSender()
	probeRTT.now = monotime.Time((probeRTT.probeRTTMinStamp + probeRTT.tuning.probeRTTWinUS + 1) * 1000)
	probeRTT.updateMinRTT(&b3RS{rttUS: 10000})
	capProbe := newSender()
	capProbe.initialized = true
	capProbe.mode = b3ProbeBW
	capProbe.cycleIdx = b3ProbeUp
	capProbe.inflightHi = 6000
	capProbe.inflightLo = b3Inf
	capProbe.cwnd = 12000
	capProbe.boundCwndForInflightModel()
	down := newSender()
	down.minRTTUS = 10000
	down.bwHi = [2]uint64{1000000, 1000000}
	down.inflightHi = 100000
	down.now = monotime.Time(1000000000)
	down.cycleMstamp = 0
	loss, _ := b.isInflightTooHigh(&b3RS{lost: 300, txInFlight: 10000, losses: 1})
	hi := newSender()
	hi.minRTTUS = 100000
	hi.bwHi = [2]uint64{1000000, 1000000}
	target := hi.targetInflight()
	hi.handleInflightTooHigh(&b3RS{txInFlight: 10000}, true)
	idle := newSender()
	idle.batchActive = true
	idle.batchTime = monotime.Now()
	idle.flush()
	busy := newSender()
	busy.inflight = 1200
	busy.batchActive = true
	busy.batchTime = monotime.Now()
	busy.flush()
	explicit := newSender()
	explicit.delivered = 2400
	explicit.inflight = 1200
	explicit.OnAppLimited(1200)
	unadmitted := newSender()
	unadmitted.pacingRate = 100
	unadmitted.inflight = 1200
	unadmitted.pacer.burstTokens = 0
	unadmitted.pacer.lumpyTokens = 0
	unadmitted.pacer.pacingLimited = true
	sendTime := monotime.Now()
	unadmitted.pacer.idealNextSend = sendTime.Add(time.Second)
	nextSend := unadmitted.pacer.idealNextSend
	unadmitted.OnPacketSent(sendTime, 1248, 1, 48, true)
	startupAck := newSender()
	startupAck.roundStart = true
	startupAck.updateAckAggregation(&b3RS{intervalUS: 1, delivered: 1200, ackedSacked: 1200})
	startupWindow := b.tuning.ackWindow
	if startupAck.extraAckedWinIdx == 1 {
		startupWindow = 1
	}
	ackCap := uint64(b3ExtraAckedMaxUS)
	if b.tuning.ackUncapped {
		ackCap = 0
	}
	rows := []struct {
		name, anchorBbr2, anchorBbr3, fork, actual, deviation string
	}{
		{"Startup pacing gain", "738", "709", "710", fmt.Sprint(b3StartupPacingGain), "D8"},
		{"Startup cwnd gain", "512", "512", "512", fmt.Sprint(b3StartupCwndGain), ""},
		{"Full-bandwidth exit", "320/3", "320/3", "320/3", fmt.Sprintf("%d/%d", b3FullBwThresh, b3FullBwCnt), ""},
		{"Startup loss exit", "8", "8", "6", fmt.Sprint(b3FullLossCnt), "D8"},
		{"Drain gain and exit", "88/BDP", "128/3", "128/3", fmt.Sprintf("%d/%d", b3DrainGain, b3MaxDrainRound), "D8"},
		{"ProbeBW pacing gains", "[320 232 256 256]", "[320 230 256 256]", "[320 232 256 256]", fmt.Sprint(b3PacingGains), ""},
		{"ProbeBW cwnd gain", "512/512", "576/512", "576/512", fmt.Sprintf("%d/%d", b.tuning.cwndGain+b3BwProbeCwndGain, b.tuning.cwndGain), "D6"},
		{"ProbeUp cap and exit", "false/3", "false/3", "true/3", fmt.Sprintf("%t/%d", capProbe.cwnd == 6000, b3FullBwCnt), "D6"},
		{"ProbeDown exit to Cruise", "true/true", "true/true", "false/true", fmt.Sprintf("%t/%t", down.checkTimeToCruise(1000000, down.maxBw()), down.checkTimeToCruise(0, down.maxBw())), "D7"},
		{"Probe wait and Reno coexistence", "2000000/1000000/2/63", "2000000/1000000/2/63", "2000000/1000000/2/63", fmt.Sprintf("%d/%d/%d/%d", b3BwProbeBaseUS, b3BwProbeRandUS, b3BwProbeRandRounds, b3BwProbeMaxRounds), ""},
		{"Loss threshold", "5/5", "5/5", "5/12", fmt.Sprintf("%d/%d", b.lossThreshold, lossProfile.lossThreshold), "D1"},
		{"Loss test in ProbeBW", "false", "false", "true", fmt.Sprint(loss), "D6"},
		{"inflight_hi on too-high", "true", "true", "true", fmt.Sprint(hi.inflightHi == max(uint64(10000), (target*(b3Unit-b3Beta))>>b3Scale)), ""},
		{"Beta", "76", "76", "76", fmt.Sprint(b3Beta), ""},
		{"Headroom", "38", "38", "38", fmt.Sprint(b.tuning.headroom), ""},
		{"bw_lo/inflight_lo", "false/false", "false/false", "false/true", fmt.Sprintf("%t/%t", b.noLower, lossProfile.noLower), "D1"},
		{"ProbeRTT interval and entry", "10000000/false", "10000000/false", "10000000/true", fmt.Sprintf("%d/%t", b.tuning.probeRTTWinUS, probeRTT.mode == b3ProbeRTT), "D2"},
		{"ProbeRTT duration and cwnd", "200000/128", "200000/128", "200000/128", fmt.Sprintf("%d/%d", b3ProbeRTTModeUS, b.tuning.probeRTTGain), ""},
		{"Min-RTT filter", "10000000/10000000", "10000000/10000000", "10000000/10000000", fmt.Sprintf("%d/%d", b3MinRTTWinUS, b.tuning.probeRTTWinUS), ""},
		{"Bandwidth filter", "2", "2", "2", fmt.Sprint(len(b.bwHi)), ""},
		{"Pacing margin", "0", "0", "0", fmt.Sprint(b.tuning.pacingMargin), ""},
		{"ACK aggregation", "10/10/true/0", "1/10/true/0", "1/10/true/0", fmt.Sprintf("%d/%d/%t/%d", startupWindow, b.tuning.ackWindow, b.tuning.ackUncapped, ackCap), "D4"},
		{"Minimum cwnd", "4", "4", "4", fmt.Sprint(b.minCwnd() / uint64(b.mss)), ""},
		{"Initial and maximum cwnd", "32/2400000", "32/2400000", "32/50400000", fmt.Sprintf("%d/%d", b.cwnd/uint64(b.mss), b.cwndClamp), "D10"},
		{"App-limited handling (idle/busy/explicit)", "false/false/true", "false/false/true", "true/false/true", fmt.Sprintf("%t/%t/%t", idle.appLimited != 0, busy.appLimited != 0, explicit.appLimited == 3600), "D9"},
		{"Unadmitted packet preserves pacer schedule", "false", "false", "true", fmt.Sprint(unadmitted.pacer.idealNextSend == nextSend), "D11"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if row.anchorBbr2 == "" || row.anchorBbr3 == "" {
				t.Fatal("missing anchor value")
			}
			if row.actual != row.fork {
				t.Errorf("sender value = %s, recorded fork value = %s", row.actual, row.fork)
			}
			if row.fork != row.anchorBbr2 && row.deviation == "" {
				t.Fatal("fork differs from Bbr2Sender without a deviation ID")
			}
		})
	}
}
