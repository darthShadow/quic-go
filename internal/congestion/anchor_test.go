//go:build !shunpike_research

package congestion

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/internal/monotime"
	"github.com/quic-go/quic-go/internal/utils"
)

type anchorMeasurement struct {
	file string
	line int
}

type anchorReason struct {
	measured anchorMeasurement
	pending  string
}

func checkAnchorMeasurement(ref anchorMeasurement) error {
	if ref.file == "" || ref.line < 1 {
		return fmt.Errorf("needs a results file and positive line number")
	}
	data, err := os.ReadFile(filepath.Join("../..", ref.file))
	if err != nil {
		return fmt.Errorf("read measurement %s: %w", ref.file, err)
	}
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	if ref.line > lines {
		return fmt.Errorf("measurement %s:%d exceeds %d lines", ref.file, ref.line, lines)
	}
	return nil
}

func TestAnchorMeasurementReferences(t *testing.T) {
	for _, row := range []struct {
		name      string
		ref       anchorMeasurement
		wantError bool
	}{
		{"valid", anchorMeasurement{"measurements/n4-results.md", 2}, false},
		{"past_end", anchorMeasurement{"measurements/n4-results.md", 99}, true},
		{"missing_file", anchorMeasurement{"measurements/missing.md", 1}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			if err := checkAnchorMeasurement(row.ref); (err != nil) != row.wantError {
				t.Fatalf("measurement %s:%d: error = %v, wantError = %t", row.ref.file, row.ref.line, err, row.wantError)
			}
		})
	}
}

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
		reason                                                anchorReason
	}{
		{"Startup pacing gain", "738", "709", "710", fmt.Sprint(b3StartupPacingGain), "D8", anchorReason{pending: "Unmeasured"}},
		{"Startup cwnd gain", "512", "512", "512", fmt.Sprint(b3StartupCwndGain), "", anchorReason{}},
		{"Full-bandwidth exit", "320/3", "320/3", "320/3", fmt.Sprintf("%d/%d", b3FullBwThresh, b3FullBwCnt), "", anchorReason{}},
		{"Startup loss exit", "8", "8", "6", fmt.Sprint(b3FullLossCnt), "D8", anchorReason{pending: "Unmeasured"}},
		{"Drain gain and exit", "88/BDP", "128/3", "128/3", fmt.Sprintf("%d/%d", b3DrainGain, b3MaxDrainRound), "D8", anchorReason{pending: "Unmeasured"}},
		{"ProbeBW pacing gains", "[320 232 256 256]", "[320 230 256 256]", "[320 232 256 256]", fmt.Sprint(b3PacingGains), "", anchorReason{}},
		{"ProbeBW cwnd gain", "512/512", "576/512", "576/512", fmt.Sprintf("%d/%d", b.tuning.cwndGain+b3BwProbeCwndGain, b.tuning.cwndGain), "D6", anchorReason{pending: "W29a measures ProbeBW Exits and ProbeUp Anchor; the 2.25 cwnd gain is not isolated"}},
		{"ProbeUp cap and exit", "false/3", "false/3", "true/3", fmt.Sprintf("%t/%d", capProbe.cwnd == 6000, b3FullBwCnt), "D6", anchorReason{measured: anchorMeasurement{file: "measurements/w29a-confirm.md", line: 28}}},
		{"ProbeDown exit to Cruise", "true/true", "true/true", "false/true", fmt.Sprintf("%t/%t", down.checkTimeToCruise(1000000, down.maxBw()), down.checkTimeToCruise(0, down.maxBw())), "D7", anchorReason{measured: anchorMeasurement{file: "measurements/w29a-confirm.md", line: 29}}},
		{"Probe wait and Reno coexistence", "2000000/1000000/2/63", "2000000/1000000/2/63", "2000000/1000000/2/63", fmt.Sprintf("%d/%d/%d/%d", b3BwProbeBaseUS, b3BwProbeRandUS, b3BwProbeRandRounds, b3BwProbeMaxRounds), "", anchorReason{}},
		{"Loss threshold", "5/5", "5/5", "5/12", fmt.Sprintf("%d/%d", b.lossThreshold, lossProfile.lossThreshold), "D1", anchorReason{measured: anchorMeasurement{file: "measurements/n4-results.md", line: 1}}},
		{"Loss test in ProbeBW", "false", "false", "true", fmt.Sprint(loss), "D6", anchorReason{measured: anchorMeasurement{file: "measurements/w29a-confirm.md", line: 32}}},
		{"inflight_hi on too-high", "true", "true", "true", fmt.Sprint(hi.inflightHi == max(uint64(10000), (target*(b3Unit-b3Beta))>>b3Scale)), "", anchorReason{}},
		{"Beta", "76", "76", "76", fmt.Sprint(b3Beta), "", anchorReason{}},
		{"Headroom", "38", "38", "38", fmt.Sprint(b.tuning.headroom), "", anchorReason{}},
		{"bw_lo/inflight_lo", "false/false", "false/false", "false/true", fmt.Sprintf("%t/%t", b.noLower, lossProfile.noLower), "D1", anchorReason{measured: anchorMeasurement{file: "measurements/n4-results.md", line: 1}}},
		{"ProbeRTT interval and entry", "10000000/false", "10000000/false", "10000000/true", fmt.Sprintf("%d/%t", b.tuning.probeRTTWinUS, probeRTT.mode == b3ProbeRTT), "D2", anchorReason{pending: "Measured only inside C2 (W22j); individual effect not isolated"}},
		{"ProbeRTT duration and cwnd", "200000/128", "200000/128", "200000/128", fmt.Sprintf("%d/%d", b3ProbeRTTModeUS, b.tuning.probeRTTGain), "", anchorReason{}},
		{"Min-RTT filter", "10000000/10000000", "10000000/10000000", "10000000/10000000", fmt.Sprintf("%d/%d", b3MinRTTWinUS, b.tuning.probeRTTWinUS), "", anchorReason{}},
		{"Bandwidth filter", "2", "2", "2", fmt.Sprint(len(b.bwHi)), "", anchorReason{}},
		{"Pacing margin", "0", "0", "0", fmt.Sprint(b.tuning.pacingMargin), "", anchorReason{}},
		{"ACK aggregation", "10/10/true/0", "1/10/true/0", "1/10/true/0", fmt.Sprintf("%d/%d/%t/%d", startupWindow, b.tuning.ackWindow, b.tuning.ackUncapped, ackCap), "D4", anchorReason{pending: "Measured only inside C2 (W22j); individual effect not isolated"}},
		{"Minimum cwnd", "4", "4", "4", fmt.Sprint(b.minCwnd() / uint64(b.mss)), "", anchorReason{}},
		{"Initial and maximum cwnd", "32/2400000", "32/2400000", "32/50400000", fmt.Sprintf("%d/%d", b.cwnd/uint64(b.mss), b.cwndClamp), "D10", anchorReason{pending: "R140 at ea20f93: 2,860 cap samples at 50,400,000 bytes; R173 doubled-cap goodput regresses at 350 ms; server memory per connection unmeasured"}},
		{"App-limited handling (idle/busy/explicit)", "false/false/true", "false/false/true", "true/false/true", fmt.Sprintf("%t/%t/%t", idle.appLimited != 0, busy.appLimited != 0, explicit.appLimited == 3600), "D9", anchorReason{pending: "R140 at ea20f93: no post-Startup stall; individual effect not isolated"}},
		{"Unadmitted packet preserves pacer schedule", "false", "false", "true", fmt.Sprint(unadmitted.pacer.idealNextSend == nextSend), "D11", anchorReason{pending: "R140 at ea20f93: no post-Startup stall; individual effect not isolated"}},
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
			if row.deviation == "" {
				return
			}
			measured := row.reason.measured != (anchorMeasurement{})
			pending := row.reason.pending != ""
			if measured == pending {
				t.Fatalf("%s needs exactly one measured reference or pending measurement ID", row.deviation)
			}
			if measured {
				if err := checkAnchorMeasurement(row.reason.measured); err != nil {
					t.Fatalf("%s: %v", row.deviation, err)
				}
			}
		})
	}
}

// Research arms record measured alternatives and promotion decisions separately
// from TestAnchorDefaults' equality check against the sender.
// This list owns the ledger; SILO.md is generated from it by the check below.
type anchorCost struct {
	baseline, arm float64
}

type anchorResearchCell struct {
	name, measurementID                  string
	measured                             anchorMeasurement
	goodputRatio                         float64
	interval95                           [2]float64
	pingP95MS                            *anchorCost
	n4Share                              *anchorCost
	competitorShare                      *anchorCost // CUBIC share; nil means unmeasured, never a passing guard
	serverCPUPerGB, totalCPUPerGB, drops anchorCost
}

type anchorResearchArm struct {
	name, patch, anchor, anchorSource, fork, shipped, status, decision string
	cells                                                              []anchorResearchCell
}

// Anchor source paths are under quiche/quic/core at 7980e3d5d89a.
var anchorResearchArms = []anchorResearchArm{
	{
		name:         "Pacer Catch-Up",
		patch:        "quic-pacer-catchup.patch",
		anchor:       "ideal send schedule; catch up while pacing-limited; 1 ms allowance; 10-packet initial burst and up to 2-packet lumps",
		anchorSource: "congestion_control/pacing_sender.cc:66-117,146-156",
		fork:         "same schedule and allowance; catch-up debt capped at 4 ms; burst budget max(5 ms of rate, 2 MSS)",
		shipped:      "BBRv3 uses the measured 4 ms quiche pacer through the Pacer + ACK4 promotion; this standalone arm retains its earlier rejection; the default sender keeps the upstream token bucket",
		status:       "not promoted (#456): competitor share fails fifo-deep-50/100; goodput bound fails fifo-deep-25; server CPU fails wan-200",
		decision:     "W29a records solo confirms; W42a adds competing CUBIC and household ping in every cell. At wan-200, both server and server+client CPU fail the guard. W42a evaluates the competitor-share guard on N4 share.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 21},
				goodputRatio: 1.0234348880257325, interval95: [2]float64{1.0228830183718223, 1.0269140155665186},
				pingP95MS:      &anchorCost{94.75, 93.2},
				serverCPUPerGB: anchorCost{23.095346612792817, 25.25174085735432},
				totalCPUPerGB:  anchorCost{58.95274059513299, 57.511122857923596},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "fifo-deep-50", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 22},
				goodputRatio: 1.035729305120751, interval95: [2]float64{1.0325145809064937, 1.0414836272471872},
				pingP95MS:      &anchorCost{98.25, 88.75},
				serverCPUPerGB: anchorCost{22.12344247647671, 23.089075433725284},
				totalCPUPerGB:  anchorCost{53.99098908389155, 52.49088010427057},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "fifo-deep-100", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 23},
				goodputRatio: 1.0417474728938192, interval95: [2]float64{1.0376610618108446, 1.0451832731329724},
				pingP95MS:      &anchorCost{98.45, 87.5},
				serverCPUPerGB: anchorCost{18.977051510733013, 20.7814945101833},
				totalCPUPerGB:  anchorCost{46.45231784304683, 45.903754692157726},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "wan-200", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 24},
				goodputRatio: 1.0406491084245921, interval95: [2]float64{1.03831907279548, 1.0445929712620863},
				serverCPUPerGB: anchorCost{17.52195912911854, 19.72151271201373},
				totalCPUPerGB:  anchorCost{37.11274887018449, 39.46989363695536},
				drops:          anchorCost{4943.0, 5129.0},
			},
			{
				name: "police-25", measurementID: "W29a/confirm-police",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 25},
				goodputRatio: 1.024194010889775, interval95: [2]float64{1.017410802620328, 1.0316688605935},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{25.43819734228181, 24.549573298237053},
				totalCPUPerGB:  anchorCost{48.16320179044253, 45.87120123375566},
				drops:          anchorCost{285.0, 458.5},
			},
			{
				name: "police-100", measurementID: "W29a/confirm-police",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 26},
				goodputRatio: 1.1253871915984979, interval95: [2]float64{1.1105903670527222, 1.1396329420436053},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{10.184706103367834, 10.443544756572175},
				totalCPUPerGB:  anchorCost{19.491199548628423, 19.80565381746632},
				drops:          anchorCost{661.0, 585.5},
			},
			{
				name: "police-50", measurementID: "W29a/confirm-police50",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 27},
				goodputRatio: 1.0699453589740011, interval95: [2]float64{1.0637951143983209, 1.0812735215613234},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{15.154407051376584, 15.337595452700352},
				totalCPUPerGB:  anchorCost{29.117327129996116, 28.899170959575322},
				drops:          anchorCost{369.0, 438.0},
			},
			{
				name: "fifo-deep-25", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 21},
				goodputRatio: 0.9550569470142447, interval95: [2]float64{0.8970650483658354, 0.9944861756635011},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.5797339681255198, 0.555845928394026},
				competitorShare: &anchorCost{0.4202660318744801, 0.444154071605974},
				serverCPUPerGB:  anchorCost{30.182815036682562, 31.39141932999846},
				totalCPUPerGB:   anchorCost{66.5115103526509, 66.50199488164235},
				drops:           anchorCost{428.5, 386.5},
			},
			{
				name: "fifo-deep-50", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 22},
				goodputRatio: 1.298443211959914, interval95: [2]float64{1.2103103084087024, 1.3426269465873015},
				pingP95MS:       &anchorCost{339.0, 340.0},
				n4Share:         &anchorCost{0.47131454952576957, 0.6125350594545043},
				competitorShare: &anchorCost{0.5286854504742304, 0.38746494054549574},
				serverCPUPerGB:  anchorCost{25.418496306115756, 25.640900423365153},
				totalCPUPerGB:   anchorCost{60.27777449422102, 56.99232204171085},
				drops:           anchorCost{501.0, 823.0},
			},
			{
				name: "fifo-deep-100", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 23},
				goodputRatio: 1.2503452316921293, interval95: [2]float64{1.1706914594125006, 1.4258310325747336},
				pingP95MS:       &anchorCost{336.5, 339.0},
				n4Share:         &anchorCost{0.4731486203533505, 0.5932515786082022},
				competitorShare: &anchorCost{0.5268513796466496, 0.4067484213917978},
				serverCPUPerGB:  anchorCost{21.5694587078211, 22.103210252270543},
				totalCPUPerGB:   anchorCost{52.26931845530826, 49.96122332002731},
				drops:           anchorCost{514.5, 921.5},
			},
			{
				name: "wan-200", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 24},
				goodputRatio: 1.0457505564722256, interval95: [2]float64{1.042473108065311, 1.0485493296556245},
				pingP95MS:       &anchorCost{71.5, 71.15},
				n4Share:         &anchorCost{0.9839116208518548, 0.985264178473217},
				competitorShare: &anchorCost{0.016088379148145232, 0.014735821526782999},
				serverCPUPerGB:  anchorCost{17.719176531604028, 20.07873308806048},
				totalCPUPerGB:   anchorCost{37.522564913235676, 40.23485371516642},
				drops:           anchorCost{4943.5, 5211.0},
			},
			{
				name: "police-25", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 25},
				goodputRatio: 1.0160093425347043, interval95: [2]float64{0.9954941507903151, 1.0235198959415108},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.9022896472802446, 0.8918949193864929},
				competitorShare: &anchorCost{0.0977103527197554, 0.10810508061350706},
				serverCPUPerGB:  anchorCost{24.748968426672818, 25.129567657790354},
				totalCPUPerGB:   anchorCost{46.68186924607471, 46.765929544976366},
				drops:           anchorCost{558.5, 456.5},
			},
			{
				name: "police-50", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 26},
				goodputRatio: 1.058014655269615, interval95: [2]float64{1.0390344174983828, 1.0875164491832834},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8736229146409322, 0.8735769746750348},
				competitorShare: &anchorCost{0.12637708535906778, 0.12642302532496524},
				serverCPUPerGB:  anchorCost{15.631969504542257, 15.92187728783206},
				totalCPUPerGB:   anchorCost{30.022076892854578, 29.938623720354666},
				drops:           anchorCost{619.0, 558.5},
			},
			{
				name: "police-100", measurementID: "W42a/guards-n10",
				measured:     anchorMeasurement{file: "measurements/w42a-guards.md", line: 27},
				goodputRatio: 1.1002644522679583, interval95: [2]float64{1.0805754846414564, 1.1212174773260624},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8804077176907352, 0.8931796020370726},
				competitorShare: &anchorCost{0.11959228230926484, 0.10682039796292742},
				serverCPUPerGB:  anchorCost{10.566044521601249, 10.738548744720553},
				totalCPUPerGB:   anchorCost{20.3969071555775, 20.45456637114922},
				drops:           anchorCost{950.5, 792.5},
			},
		},
	},
	{
		name:         "ProbeBW Exits",
		patch:        "quic-probebw-exits.patch",
		anchor:       "ProbeUp: after one round, inflight >= 1.25 BDP + 2 MSS + max ACK height; ProbeDown: one min_rtt or drained; ProbeRTT: only after ProbeDown, 200 ms at 0.5 BDP, no extra round",
		anchorSource: "congestion_control/bbr2_probe_bw.cc:52-65,129-132,255-263,429-468; congestion_control/bbr2_probe_rtt.cc:26-41",
		fork:         "anchor queue and phase exits, including deferred ProbeRTT entry and no extra round",
		shipped:      "n4-c2 keeps three-round bandwidth plateau ProbeUp exit, drained-only ProbeDown exit, ProbeRTT entry from any state and 200 ms plus one round",
		status:       "not promoted: ping guard",
		decision:     "Household ping p95 rises above +5% at fifo-deep-50/100. Competitor share is unmeasured.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 28},
				goodputRatio: 1.0267447690552856, interval95: [2]float64{1.0266458359816522, 1.0290870644890102},
				pingP95MS:      &anchorCost{94.75, 94.0},
				serverCPUPerGB: anchorCost{23.095346612792817, 23.609290167379946},
				totalCPUPerGB:  anchorCost{58.95274059513299, 55.973379668097564},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "fifo-deep-50", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 29},
				goodputRatio: 1.0373577577811923, interval95: [2]float64{1.0337064085381003, 1.0420762774596692},
				pingP95MS:      &anchorCost{98.25, 120.5},
				serverCPUPerGB: anchorCost{22.12344247647671, 21.256780479382677},
				totalCPUPerGB:  anchorCost{53.99098908389155, 52.5935860634288},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "fifo-deep-100", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 30},
				goodputRatio: 1.0310590604135805, interval95: [2]float64{1.0291037242513699, 1.0338615060274663},
				pingP95MS:      &anchorCost{98.45, 119.0},
				serverCPUPerGB: anchorCost{18.977051510733013, 18.655318846114767},
				totalCPUPerGB:  anchorCost{46.45231784304683, 45.815044284870886},
				drops:          anchorCost{0.0, 0.0},
			},
			{
				name: "wan-200", measurementID: "W29a/confirm-deep-wan",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 31},
				goodputRatio: 1.0160361082638645, interval95: [2]float64{1.0132884174102734, 1.0177137364726565},
				serverCPUPerGB: anchorCost{17.52195912911854, 17.42782482521032},
				totalCPUPerGB:  anchorCost{37.11274887018449, 36.940734128816615},
				drops:          anchorCost{4943.0, 5046.5},
			},
		},
	},
	{
		name:         "ProbeUp Anchor",
		patch:        "quic-probeup-anchor.patch",
		anchor:       "ProbeUp ignores inflight_hi and slope growth; per-ACK round loss test: >=2 loss events and >2% of latest ACKed packet send-state inflight; inflight_hi = max(that inflight, 0.7 x target)",
		anchorSource: "congestion_control/bbr2_probe_bw.cc:76-87,172-218,321-327; congestion_control/bbr2_misc.cc:163-166,323-328,354-386,394-396",
		fork:         "anchor ProbeUp cap, loss test and send-state inflight_hi; Startup stays unchanged",
		shipped:      "n4-c2 keeps ProbeUp inflight_hi cap and slope growth, per-lost-packet loss test and lost-prefix inflight estimate",
		status:       "rejected: no robust gain at police-100; drop cost",
		decision:     "Police-25 gains with a lower 95% bound above 1, but drops rise from 285 to 1029 (3.61x). Police-100 has no robust gain. Competitor share is unmeasured.",
		cells: []anchorResearchCell{
			{
				name: "police-25", measurementID: "W29a/confirm-police",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 32},
				goodputRatio: 1.0138288445388144, interval95: [2]float64{1.0030033756101946, 1.0230602922257908},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{25.43819734228181, 22.744889155920873},
				totalCPUPerGB:  anchorCost{48.16320179044253, 43.50808619449186},
				drops:          anchorCost{285.0, 1029.0},
			},
			{
				name: "police-100", measurementID: "W29a/confirm-police",
				measured:     anchorMeasurement{file: "measurements/w29a-confirm.md", line: 33},
				goodputRatio: 1.008645492384346, interval95: [2]float64{0.9914901334493179, 1.029429308416948},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{10.184706103367834, 10.693970036869565},
				totalCPUPerGB:  anchorCost{19.491199548628423, 21.017458463699555},
				drops:          anchorCost{661.0, 1636.0},
			},
		},
	},
	{
		name:         "Pacer + ACK4",
		patch:        "quic-pacer-ack4.patch",
		anchor:       "receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance",
		anchorSource: "quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156",
		fork:         "client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 4 ms, burst budget max(5 ms of rate, 2 MSS)",
		shipped:      "Config.ACKDecimation opts either endpoint into the measured receiver policy, off by default; BBRv3 always uses the measured quiche pacer with 4 ms catch-up debt, initial and idle bursts, two-packet lumps and max(5 ms of rate, 2 MSS) budget; the default sender retains the upstream token bucket",
		status:       "promoted by user decision R55 after W51a confirm; share and household-ping trade-offs accepted",
		decision:     "W51a confirms n4-pacer-ack4 against n4-c2 with n=30 in each deep-FIFO cell and n=10 elsewhere. R55 (Promote anyway) accepts N4-share ratios 1.229 at fifo-deep-50 and 1.291 at fifo-deep-100 against a competing CUBIC download, and household ping ratio 1.21 at wan-1000. These ratios are the approved promotion record; the full W51a per-cell output is not present in this checkout. R56 selects one receiver config field and the pacer in BBRv3. W37a, W42b and W43a remain the earlier screen and confirm records below; their failed guards are not erased.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 20},
				goodputRatio: 0.9903979292404435, interval95: [2]float64{0.9071039358481847, 1.0264208606482625},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.5756753726779993, 0.5735516985580433},
				competitorShare: &anchorCost{0.4243246273220006, 0.42644830144195667},
				serverCPUPerGB:  anchorCost{31.58291221422337, 27.511706734298635},
				totalCPUPerGB:   anchorCost{69.12751422877567, 58.565609013315665},
				drops:           anchorCost{372.0, 319.0},
			},
			{
				name: "fifo-deep-50", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 21},
				goodputRatio: 1.2069039948317333, interval95: [2]float64{1.1272278926471795, 1.3057208486806602},
				pingP95MS:       &anchorCost{338.5, 339.0},
				n4Share:         &anchorCost{0.47055238030507773, 0.5698981433924641},
				competitorShare: &anchorCost{0.5294476196949223, 0.4301018566075358},
				serverCPUPerGB:  anchorCost{26.2492331887178, 19.16109615532727},
				totalCPUPerGB:   anchorCost{61.97451276868122, 47.46046069191061},
				drops:           anchorCost{459.5, 854.5},
			},
			{
				name: "fifo-deep-100", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 22},
				goodputRatio: 1.2474531034911238, interval95: [2]float64{1.1234645238709808, 1.3037049899888848},
				pingP95MS:       &anchorCost{337.5, 340.0},
				n4Share:         &anchorCost{0.49192497262617496, 0.6141657438246314},
				competitorShare: &anchorCost{0.508075027373825, 0.38583425617536854},
				serverCPUPerGB:  anchorCost{21.902009635913508, 14.6891014537747},
				totalCPUPerGB:   anchorCost{52.91060600393969, 39.50546245533222},
				drops:           anchorCost{608.0, 1119.0},
			},
			{
				name: "wan-200", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 23},
				goodputRatio: 1.042149780470129, interval95: [2]float64{1.0398903005286113, 1.0451560604629726},
				pingP95MS:       &anchorCost{70.55, 70.15},
				n4Share:         &anchorCost{0.9853865304257032, 0.9856097321833872},
				competitorShare: &anchorCost{0.01461346957429679, 0.014390267816612876},
				serverCPUPerGB:  anchorCost{18.099488998270374, 13.25514459441958},
				totalCPUPerGB:   anchorCost{38.22723096893036, 30.51123984162336},
				drops:           anchorCost{4956.0, 5220.5},
			},
			{
				name: "police-25", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 24},
				goodputRatio: 1.0206051209685205, interval95: [2]float64{1.0015784269619272, 1.042997259445619},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8860061938923209, 0.8883133048169342},
				competitorShare: &anchorCost{0.11399380610767901, 0.11168669518306587},
				serverCPUPerGB:  anchorCost{25.276818508898437, 23.32873641639093},
				totalCPUPerGB:   anchorCost{47.90958344635536, 42.64348975301952},
				drops:           anchorCost{502.5, 447.5},
			},
			{
				name: "police-50", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 25},
				goodputRatio: 1.0689364605050509, interval95: [2]float64{1.0429791478534367, 1.0849494636453942},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8811613780780869, 0.8875821557439127},
				competitorShare: &anchorCost{0.11883862192191316, 0.11241784425608733},
				serverCPUPerGB:  anchorCost{15.723230754574105, 15.418482399359714},
				totalCPUPerGB:   anchorCost{30.24959552696457, 28.86879257882833},
				drops:           anchorCost{621.5, 591.0},
			},
			{
				name: "police-100", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 26},
				goodputRatio: 1.1059245976243417, interval95: [2]float64{1.068615211630116, 1.123288409800246},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8773997045172679, 0.8891222252151454},
				competitorShare: &anchorCost{0.12260029548273206, 0.11087777478485464},
				serverCPUPerGB:  anchorCost{10.801199174496592, 10.12072824550826},
				totalCPUPerGB:   anchorCost{20.873123652803468, 19.259236717013415},
				drops:           anchorCost{868.0, 805.5},
			},
			{
				name: "wan-1000", measurementID: "W42b/confirm-n10",
				measured:     anchorMeasurement{file: "measurements/w42b-confirm.md", line: 27},
				goodputRatio: 1.1465000649602122, interval95: [2]float64{1.117554015184012, 1.161462029238786},
				pingP95MS:       &anchorCost{59.650000000000006, 65.25},
				n4Share:         &anchorCost{0.9964711871374905, 0.9975630474711232},
				competitorShare: &anchorCost{0.003528812862509434, 0.0024369525288767103},
				serverCPUPerGB:  anchorCost{13.063059514546199, 13.014795946214221},
				totalCPUPerGB:   anchorCost{25.648232017834275, 23.882669073389664},
				drops:           anchorCost{27059.5, 31698.0},
			},
			{
				name: "fifo-deep-25", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 19},
				goodputRatio: 1.0267546141653565, interval95: [2]float64{1.024970391902966, 1.0286010055469157},
				pingP95MS:      &anchorCost{95.8, 94.8},
				serverCPUPerGB: anchorCost{22.26271497958666, 20.2046171523011},
				totalCPUPerGB:  anchorCost{56.77678850638734, 47.49301370418788},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-25", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 20},
				goodputRatio: 1.0286561849570368, interval95: [2]float64{1.0233175834792454, 1.0352838130117332},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{24.416950323142853, 21.434651264223305},
				totalCPUPerGB:  anchorCost{46.11337595198861, 39.31661107104499},
				drops:          anchorCost{350, 423},
			},
			{
				name: "fifo-deep-50", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 21},
				goodputRatio: 1.0412359984632191, interval95: [2]float64{1.032996944804059, 1.0448517633936454},
				pingP95MS:      &anchorCost{99.1, 91.0},
				serverCPUPerGB: anchorCost{21.1845131083099, 13.85840683596632},
				totalCPUPerGB:  anchorCost{52.08205943490972, 38.30788897784792},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-50", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 22},
				goodputRatio: 1.0582610411056215, interval95: [2]float64{1.0549976199577151, 1.0750408913250586},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{14.704604079546097, 13.934854474047178},
				totalCPUPerGB:  anchorCost{28.160765894736485, 26.29825799048796},
				drops:          anchorCost{378, 405},
			},
			{
				name: "fifo-deep-100", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 23},
				goodputRatio: 1.0425935393865855, interval95: [2]float64{1.0371946252061641, 1.0452357234552974},
				pingP95MS:      &anchorCost{98.5, 86.8},
				serverCPUPerGB: anchorCost{18.459704721924936, 12.752245278778078},
				totalCPUPerGB:  anchorCost{45.14003495141618, 34.0221873799087},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-100", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 24},
				goodputRatio: 1.1358065706840974, interval95: [2]float64{1.1130259083745337, 1.1473317278666886},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{9.805492874433522, 9.17890317891453},
				totalCPUPerGB:  anchorCost{19.01544617933839, 17.467664886723572},
				drops:          anchorCost{602, 576},
			},
			{
				name: "wan-200", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 25},
				goodputRatio: 1.0401073378934222, interval95: [2]float64{1.038474348413553, 1.0450360447980922},
				serverCPUPerGB: anchorCost{16.916284691798513, 12.092623782173902},
				totalCPUPerGB:  anchorCost{35.87104931170632, 28.283490418529876},
				drops:          anchorCost{5042, 5127},
			},
			{
				name: "wan-1000", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 26},
				goodputRatio: 1.1191032413380804, interval95: [2]float64{1.1168237517560267, 1.12497338165711},
				serverCPUPerGB: anchorCost{12.236750745227479, 11.841268524516337},
				totalCPUPerGB:  anchorCost{24.094476054019076, 22.101432878750877},
				drops:          anchorCost{26996, 31572},
			},
			{
				name: "fifo-deep-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 21},
				goodputRatio: 0.9371264478910373, interval95: [2]float64{0.7184949394614429, 1.1113527140952277},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.5988899525139313, 0.5622242778365377},
				competitorShare: &anchorCost{0.40111004748606877, 0.4377757221634622},
				serverCPUPerGB:  anchorCost{29.26603870377338, 27.425969650662214},
				totalCPUPerGB:   anchorCost{65.41339060775732, 57.61355900078502},
				drops:           anchorCost{397, 260},
			},
			{
				name: "fifo-deep-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 22},
				goodputRatio: 1.2435066171757638, interval95: [2]float64{1.0779596354894752, 1.3081984297926619},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.46660231099586064, 0.5806362152984581},
				competitorShare: &anchorCost{0.5333976890041393, 0.41936378470154184},
				serverCPUPerGB:  anchorCost{25.378138452732728, 18.996637336146446},
				totalCPUPerGB:   anchorCost{60.13538562984649, 46.283165869260955},
				drops:           anchorCost{510, 755},
			},
			{
				name: "fifo-deep-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 23},
				goodputRatio: 1.4154917634025004, interval95: [2]float64{1.116215464450646, 2.773126612733746},
				pingP95MS:       &anchorCost{335.0, 339.0},
				n4Share:         &anchorCost{0.484822980884167, 0.6944242943728326},
				competitorShare: &anchorCost{0.515177019115833, 0.3055757056271674},
				serverCPUPerGB:  anchorCost{21.408615495179816, 14.2431980770948},
				totalCPUPerGB:   anchorCost{51.77720529663044, 38.42129176193973},
				drops:           anchorCost{352, 1444},
			},
			{
				name: "wan-200", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 24},
				goodputRatio: 1.04505734024152, interval95: [2]float64{1.038992157442173, 1.0492890581025154},
				pingP95MS:       &anchorCost{70.5, 72.2},
				n4Share:         &anchorCost{0.9857547807965705, 0.986145579225204},
				competitorShare: &anchorCost{0.014245219203429433, 0.013854420774796053},
				serverCPUPerGB:  anchorCost{17.606127353549745, 12.883845670683712},
				totalCPUPerGB:   anchorCost{37.23372285216831, 29.75285440236324},
				drops:           anchorCost{4991, 5241},
			},
			{
				name: "police-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 25},
				goodputRatio: 0.9947444210793933, interval95: [2]float64{0.9648527772232725, 1.0284870458089432},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.9004129358799098, 0.8763842713904259},
				competitorShare: &anchorCost{0.09958706412009025, 0.12361572860957418},
				serverCPUPerGB:  anchorCost{25.370763286403378, 23.13216030319785},
				totalCPUPerGB:   anchorCost{47.72861898987652, 42.06284423756465},
				drops:           anchorCost{562, 440},
			},
			{
				name: "police-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 26},
				goodputRatio: 1.0682159463683867, interval95: [2]float64{1.0413703580461517, 1.0994865408329093},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8770782280890981, 0.8862489587278659},
				competitorShare: &anchorCost{0.12292177191090196, 0.11375104127213412},
				serverCPUPerGB:  anchorCost{15.74655825104009, 15.255887287135875},
				totalCPUPerGB:   anchorCost{30.036705055304708, 28.40858996419832},
				drops:           anchorCost{608, 598},
			},
			{
				name: "police-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 27},
				goodputRatio: 1.1041205165244508, interval95: [2]float64{1.0589497000510404, 1.1587029251499283},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8717105981467959, 0.8846181731472975},
				competitorShare: &anchorCost{0.12828940185320414, 0.11538182685270244},
				serverCPUPerGB:  anchorCost{10.411251083757978, 9.740763589525361},
				totalCPUPerGB:   anchorCost{20.263627583762414, 18.679836446560124},
				drops:           anchorCost{856, 805},
			},
			{
				name: "wan-1000", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 28},
				goodputRatio: 1.124291584253623, interval95: [2]float64{1.1161348537412528, 1.1471771847818062},
				pingP95MS:       &anchorCost{61.3, 65.4},
				n4Share:         &anchorCost{0.9965271035838864, 0.9978776553260787},
				competitorShare: &anchorCost{0.003472896416113623, 0.0021223446739212478},
				serverCPUPerGB:  anchorCost{12.877602045037081, 12.770190024347635},
				totalCPUPerGB:   anchorCost{25.269640794289344, 23.504350333820277},
				drops:           anchorCost{27028, 32314},
			},
		},
	},
	{
		name:         "Client ACK4",
		patch:        "quic-client-ack4.patch",
		anchor:       "receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4)",
		anchorSource: "quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303",
		fork:         "client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); sender pacing stays unchanged",
		shipped:      "Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 includes the promoted 4 ms quiche pacer, so the receiver-only experimental combination is not the shipped BBRv3 policy",
		status:       "rejected at screen: wan-1000 lower bound 0.918",
		decision:     "W37a screen only; no n=10 confirm. Competition shares and WAN household ping are unmeasured in this screen.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 27},
				goodputRatio: 0.9766430069647848, interval95: [2]float64{0.9754521699630421, 0.987104413835736},
				pingP95MS:      &anchorCost{95.8, 94.4},
				serverCPUPerGB: anchorCost{22.26271497958666, 21.813467768503465},
				totalCPUPerGB:  anchorCost{56.77678850638734, 52.68043093139674},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-25", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 28},
				goodputRatio: 1.0293722890490649, interval95: [2]float64{1.0233936002273878, 1.0351699500599012},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{24.416950323142853, 21.103266480697418},
				totalCPUPerGB:  anchorCost{46.11337595198861, 38.54007227633439},
				drops:          anchorCost{350, 384},
			},
			{
				name: "fifo-deep-50", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 29},
				goodputRatio: 0.972197648764322, interval95: [2]float64{0.9645048792026658, 0.9799048218995708},
				pingP95MS:      &anchorCost{99.1, 97.7},
				serverCPUPerGB: anchorCost{21.1845131083099, 12.801937254671879},
				totalCPUPerGB:  anchorCost{52.08205943490972, 40.12078738104281},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-50", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 30},
				goodputRatio: 0.9768366423278533, interval95: [2]float64{0.9640149713067908, 1.00076485433159},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{14.704604079546097, 14.200724940186888},
				totalCPUPerGB:  anchorCost{28.160765894736485, 27.318418260252074},
				drops:          anchorCost{378, 361},
			},
			{
				name: "fifo-deep-100", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 31},
				goodputRatio: 0.9647260519274731, interval95: [2]float64{0.9606252935651253, 0.9720088874972072},
				pingP95MS:      &anchorCost{98.5, 98.7},
				serverCPUPerGB: anchorCost{18.459704721924936, 12.104327862079327},
				totalCPUPerGB:  anchorCost{45.14003495141618, 36.00987715598865},
				drops:          anchorCost{0, 0},
			},
			{
				name: "police-100", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 32},
				goodputRatio: 1.0045912799358887, interval95: [2]float64{0.9793695277263492, 1.0140696614288152},
				pingP95MS:      &anchorCost{40.0, 40.0},
				serverCPUPerGB: anchorCost{9.805492874433522, 9.11145957382059},
				totalCPUPerGB:  anchorCost{19.01544617933839, 17.683228352888683},
				drops:          anchorCost{602, 604},
			},
			{
				name: "wan-200", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 33},
				goodputRatio: 0.976616204931461, interval95: [2]float64{0.9750072543686957, 0.9825653134648896},
				serverCPUPerGB: anchorCost{16.916284691798513, 11.058658217483167},
				totalCPUPerGB:  anchorCost{35.87104931170632, 27.538581931392756},
				drops:          anchorCost{5042, 4833},
			},
			{
				name: "wan-1000", measurementID: "W37a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w37a-screen.md", line: 34},
				goodputRatio: 0.9769893505856636, interval95: [2]float64{0.9180429882771055, 0.9991418389340133},
				serverCPUPerGB: anchorCost{12.236750745227479, 9.519253507504315},
				totalCPUPerGB:  anchorCost{24.094476054019076, 19.774046977476125},
				drops:          anchorCost{26996, 25725},
			},
		},
	},
	{
		name:         "Pacer + ACK4 (1 ms Debt)",
		patch:        "quic-pacer-ack4-1ms.patch",
		anchor:       "receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance",
		anchorSource: "quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156",
		shipped:      "Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 uses the promoted 4 ms debt cap, not this 1 ms variant; the default sender retains the upstream token bucket",
		fork:         "client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 1 ms, burst budget max(2 ms of rate, 2 MSS)",
		status:       "rejected (#456): competitor share fails fifo-deep-50/100; household ping fails wan-1000",
		decision:     "W43a n=5 screen only; also fails goodput and competitor share at fifo-deep-25. The competition guard uses N4 share.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 29},
				goodputRatio: 0.9467162538468539, interval95: [2]float64{0.8164828716944792, 1.1227254129456636},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.5988899525139313, 0.5669133794225295},
				competitorShare: &anchorCost{0.40111004748606877, 0.4330866205774705},
				serverCPUPerGB:  anchorCost{29.26603870377338, 26.960931144133767},
				totalCPUPerGB:   anchorCost{65.41339060775732, 57.41882890625601},
				drops:           anchorCost{397, 419},
			},
			{
				name: "fifo-deep-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 30},
				goodputRatio: 1.1696370891031063, interval95: [2]float64{1.013924295061748, 1.3029585507487134},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.46660231099586064, 0.546008067465114},
				competitorShare: &anchorCost{0.5333976890041393, 0.453991932534886},
				serverCPUPerGB:  anchorCost{25.378138452732728, 19.344821176873968},
				totalCPUPerGB:   anchorCost{60.13538562984649, 46.73131458664467},
				drops:           anchorCost{510, 656},
			},
			{
				name: "fifo-deep-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 31},
				goodputRatio: 1.3109770457216414, interval95: [2]float64{1.1007959865130745, 2.568369119601849},
				pingP95MS:       &anchorCost{335.0, 339.0},
				n4Share:         &anchorCost{0.484822980884167, 0.6362451963517595},
				competitorShare: &anchorCost{0.515177019115833, 0.3637548036482406},
				serverCPUPerGB:  anchorCost{21.408615495179816, 14.14487034316112},
				totalCPUPerGB:   anchorCost{51.77720529663044, 38.343966636622966},
				drops:           anchorCost{352, 1139},
			},
			{
				name: "wan-200", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 32},
				goodputRatio: 1.0430939853647394, interval95: [2]float64{1.037040197257118, 1.0486897395254418},
				pingP95MS:       &anchorCost{70.5, 70.1},
				n4Share:         &anchorCost{0.9857547807965705, 0.9852501700688192},
				competitorShare: &anchorCost{0.014245219203429433, 0.014749829931180783},
				serverCPUPerGB:  anchorCost{17.606127353549745, 12.757818083588207},
				totalCPUPerGB:   anchorCost{37.23372285216831, 29.70286125074321},
				drops:           anchorCost{4991, 5219},
			},
			{
				name: "police-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 33},
				goodputRatio: 1.0011115273869646, interval95: [2]float64{0.9870536409568138, 1.0350701300845202},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.9004129358799098, 0.8815428451285002},
				competitorShare: &anchorCost{0.09958706412009025, 0.11845715487149977},
				serverCPUPerGB:  anchorCost{25.370763286403378, 22.949545462978648},
				totalCPUPerGB:   anchorCost{47.72861898987652, 41.82936823027919},
				drops:           anchorCost{562, 453},
			},
			{
				name: "police-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 34},
				goodputRatio: 1.0628302729847554, interval95: [2]float64{1.0428685285169619, 1.093943209057386},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8770782280890981, 0.8821743567976477},
				competitorShare: &anchorCost{0.12292177191090196, 0.1178256432023523},
				serverCPUPerGB:  anchorCost{15.74655825104009, 15.294124231763124},
				totalCPUPerGB:   anchorCost{30.036705055304708, 28.412173652722913},
				drops:           anchorCost{608, 576},
			},
			{
				name: "police-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 35},
				goodputRatio: 1.0922734802115839, interval95: [2]float64{1.0501618937626758, 1.1365635509552199},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8717105981467959, 0.8803858619689012},
				competitorShare: &anchorCost{0.12828940185320414, 0.11961413803109877},
				serverCPUPerGB:  anchorCost{10.411251083757978, 9.85750395543548},
				totalCPUPerGB:   anchorCost{20.263627583762414, 18.795922356429948},
				drops:           anchorCost{856, 777},
			},
			{
				name: "wan-1000", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 36},
				goodputRatio: 1.121954305912242, interval95: [2]float64{1.114851576890376, 1.1447923297982152},
				pingP95MS:       &anchorCost{61.3, 67.1},
				n4Share:         &anchorCost{0.9965271035838864, 0.9975134240347411},
				competitorShare: &anchorCost{0.003472896416113623, 0.0024865759652589275},
				serverCPUPerGB:  anchorCost{12.877602045037081, 12.878146403035858},
				totalCPUPerGB:   anchorCost{25.269640794289344, 23.612651698958864},
				drops:           anchorCost{27028, 31907},
			},
		},
	},
	{
		name:         "Pacer + ACK4 (No Debt)",
		patch:        "quic-pacer-ack4-nodebt.patch",
		anchor:       "receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance",
		anchorSource: "quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156",
		shipped:      "Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 uses the promoted 4 ms debt cap, not this no-debt variant; the default sender retains the upstream token bucket",
		fork:         "client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 0 ms, burst budget max(1 ms of rate, 2 MSS)",
		status:       "rejected (#456): competitor share fails fifo-deep-50/100; household ping fails wan-1000",
		decision:     "W43a n=5 screen only; also fails goodput and competitor share at fifo-deep-25. The police-25 goodput bound also fails. The competition guard uses N4 share.",
		cells: []anchorResearchCell{
			{
				name: "fifo-deep-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 37},
				goodputRatio: 0.932288826688905, interval95: [2]float64{0.7399275239326029, 1.1056157044687749},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.5988899525139313, 0.5583743239782312},
				competitorShare: &anchorCost{0.40111004748606877, 0.4416256760217688},
				serverCPUPerGB:  anchorCost{29.26603870377338, 26.88214794518299},
				totalCPUPerGB:   anchorCost{65.41339060775732, 57.261382270465944},
				drops:           anchorCost{397, 393},
			},
			{
				name: "fifo-deep-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 38},
				goodputRatio: 1.30116920559477, interval95: [2]float64{1.1279456523992233, 1.3854287499558624},
				pingP95MS:       &anchorCost{339.0, 339.0},
				n4Share:         &anchorCost{0.46660231099586064, 0.6076182845952739},
				competitorShare: &anchorCost{0.5333976890041393, 0.39238171540472605},
				serverCPUPerGB:  anchorCost{25.378138452732728, 18.37320452590563},
				totalCPUPerGB:   anchorCost{60.13538562984649, 45.62888399964683},
				drops:           anchorCost{510, 975},
			},
			{
				name: "fifo-deep-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 39},
				goodputRatio: 1.334382451678401, interval95: [2]float64{1.248103128347409, 2.614223257237033},
				pingP95MS:       &anchorCost{335.0, 339.0},
				n4Share:         &anchorCost{0.484822980884167, 0.6544454625863323},
				competitorShare: &anchorCost{0.515177019115833, 0.3455545374136677},
				serverCPUPerGB:  anchorCost{21.408615495179816, 14.010129960785738},
				totalCPUPerGB:   anchorCost{51.77720529663044, 38.02223239384628},
				drops:           anchorCost{352, 1144},
			},
			{
				name: "wan-200", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 40},
				goodputRatio: 1.0440336612211458, interval95: [2]float64{1.0379744195315805, 1.048754191802223},
				pingP95MS:       &anchorCost{70.5, 71.7},
				n4Share:         &anchorCost{0.9857547807965705, 0.9856086298532933},
				competitorShare: &anchorCost{0.014245219203429433, 0.014391370146706607},
				serverCPUPerGB:  anchorCost{17.606127353549745, 12.728400406606594},
				totalCPUPerGB:   anchorCost{37.23372285216831, 29.735429624069333},
				drops:           anchorCost{4991, 5237},
			},
			{
				name: "police-25", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 41},
				goodputRatio: 0.9961162537509318, interval95: [2]float64{0.9336222764563087, 1.0299054122775522},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.9004129358799098, 0.8787377475832664},
				competitorShare: &anchorCost{0.09958706412009025, 0.12126225241673359},
				serverCPUPerGB:  anchorCost{25.370763286403378, 22.89980574660848},
				totalCPUPerGB:   anchorCost{47.72861898987652, 41.649488568316535},
				drops:           anchorCost{562, 477},
			},
			{
				name: "police-50", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 42},
				goodputRatio: 1.0531356807648142, interval95: [2]float64{1.0333646133122991, 1.0839648206042587},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8770782280890981, 0.8717668415167829},
				competitorShare: &anchorCost{0.12292177191090196, 0.12823315848321712},
				serverCPUPerGB:  anchorCost{15.74655825104009, 15.148350341383093},
				totalCPUPerGB:   anchorCost{30.036705055304708, 28.322629043213873},
				drops:           anchorCost{608, 573},
			},
			{
				name: "police-100", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 43},
				goodputRatio: 1.1230628896608312, interval95: [2]float64{1.0636374849900154, 1.1633935073549022},
				pingP95MS:       &anchorCost{40.0, 40.0},
				n4Share:         &anchorCost{0.8717105981467959, 0.9003811356156697},
				competitorShare: &anchorCost{0.12828940185320414, 0.09961886438433029},
				serverCPUPerGB:  anchorCost{10.411251083757978, 9.820783018201798},
				totalCPUPerGB:   anchorCost{20.263627583762414, 18.675227225107733},
				drops:           anchorCost{856, 867},
			},
			{
				name: "wan-1000", measurementID: "W43a/screen-n5",
				measured:     anchorMeasurement{file: "measurements/w43a-screen.md", line: 44},
				goodputRatio: 1.1236876678521897, interval95: [2]float64{1.1171870415477516, 1.1465609753153807},
				pingP95MS:       &anchorCost{61.3, 64.4},
				n4Share:         &anchorCost{0.9965271035838864, 0.997689120950509},
				competitorShare: &anchorCost{0.003472896416113623, 0.002310879049491064},
				serverCPUPerGB:  anchorCost{12.877602045037081, 12.867617363886783},
				totalCPUPerGB:   anchorCost{25.269640794289344, 23.613025958118225},
				drops:           anchorCost{27028, 31471},
			},
		},
	},
}

var updateAnchorResearch = flag.Bool("update-anchor-research", false, "render the typed research ledger in SILO.md")

func TestAnchorResearchLedger(t *testing.T) {
	for _, arm := range anchorResearchArms {
		t.Run(arm.name, func(t *testing.T) {
			if arm.name == "" || arm.patch == "" || arm.anchor == "" || arm.anchorSource == "" || arm.fork == "" || arm.shipped == "" || arm.status == "" || arm.decision == "" || len(arm.cells) == 0 {
				t.Fatal("research arm needs anchor, fork, shipped value, status, decision and measured cells")
			}
			for _, cell := range arm.cells {
				if cell.name == "" || cell.measurementID == "" {
					t.Fatal("research cell needs a name and measurement ID")
				}
				if err := checkAnchorMeasurement(cell.measured); err != nil {
					t.Fatalf("%s: %v", cell.name, err)
				}
			}
		})
	}
	if t.Failed() {
		return
	}
	// This is a generator check: the Markdown is a view of the typed ledger.
	path := "../../SILO.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(data, []byte("## Research Arms\n"))
	end := bytes.Index(data, []byte("## Upstream Test Fixture\n"))
	if end < 0 || (start >= 0 && start >= end) {
		t.Fatal("SILO.md needs an ordered research section before the upstream fixture")
	}
	if start < 0 {
		start = end
	}
	rendered := renderAnchorResearch()
	if *updateAnchorResearch {
		updated := append([]byte(nil), data[:start]...)
		updated = append(updated, rendered...)
		updated = append(updated, data[end:]...)
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			t.Fatal(err)
		}
	} else if string(data[start:end]) != rendered {
		t.Fatal("SILO.md research ledger is stale; run go test ./internal/congestion -run '^TestAnchorResearchLedger$' -update-anchor-research")
	}
}

func renderAnchorResearch() string {
	var out strings.Builder
	out.WriteString("## Research Arms\n\n")
	out.WriteString("Generated from `anchorResearchArms` in `internal/congestion/anchor_test.go`.\n")
	out.WriteString("W29a, W42a and W42b are n=10 confirms; W37a and W43a are n=5 screens. W51a confirms\n")
	out.WriteString("Pacer + ACK4 with n=30 in deep-FIFO cells and n=10 elsewhere; R55 promotes it and R56 sets\n")
	out.WriteString("the shipped config and pacer placement. Measurement IDs keep the phases separate. Ratios compare with\n")
	out.WriteString("`n4-c2`, not Google quiche. Anchor sources below use quiche `7980e3d5d89a`,\n")
	out.WriteString("under `quiche/quic/core/`. Unmeasured costs do not establish a guard pass.\n\n")
	out.WriteString("The baseline is C2 with the explicit Silo loss profile: lower bounds off,\n")
	out.WriteString("5 percent loss threshold. The default sender retains its own loss policy.\n\n")
	out.WriteString("Costs show baseline -> arm. CPU is process seconds per decimal GB in the\n")
	out.WriteString("application measurement window. Server+client is the sum of the two reported\n")
	out.WriteString("medians, not the median of per-run sums. Drops include configured random loss\n")
	out.WriteString("in WAN cells. W29a and W37a omit competition shares and WAN household ping;\n")
	out.WriteString("W42a, W42b and W43a measure both in every cell with a competing CUBIC download. Their share\n")
	out.WriteString("guard uses N4 bytes / (N4 bytes + CUBIC bytes); both shares appear below.\n")
	out.WriteString("Each row cites its evidence snapshot under `measurements/`.\n\n")
	for _, arm := range anchorResearchArms {
		fmt.Fprintf(&out, "### %s\n\n", arm.name)
		fmt.Fprintf(&out, "- Overlay: `%s`.\n- Anchor: %s (`%s`).\n- Experimental value: %s.\n- Shipped value: %s.\n- Status: **%s**. %s\n\n", arm.patch, arm.anchor, arm.anchorSource, arm.fork, arm.shipped, arm.status, arm.decision)
		out.WriteString("| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |\n")
		out.WriteString("|---|---|---|---|---|---|---|---|---|\n")
		for _, cell := range arm.cells {
			ping, n4Share, share := "unmeasured", "unmeasured", "unmeasured"
			if cell.pingP95MS != nil {
				ping = fmt.Sprintf("%.3f -> %.3f", cell.pingP95MS.baseline, cell.pingP95MS.arm)
			}
			if cell.n4Share != nil {
				n4Share = fmt.Sprintf("%.6f -> %.6f", cell.n4Share.baseline, cell.n4Share.arm)
			}
			if cell.competitorShare != nil {
				share = fmt.Sprintf("%.6f -> %.6f", cell.competitorShare.baseline, cell.competitorShare.arm)
			}
			fmt.Fprintf(&out, "| %s | %.6f [%.6f, %.6f] | %s | %s | %s | %.6f -> %.6f | %.6f -> %.6f | %.1f -> %.1f | %s; `%s:%d` |\n", cell.name, cell.goodputRatio, cell.interval95[0], cell.interval95[1], ping, n4Share, share, cell.serverCPUPerGB.baseline, cell.serverCPUPerGB.arm, cell.totalCPUPerGB.baseline, cell.totalCPUPerGB.arm, cell.drops.baseline, cell.drops.arm, cell.measurementID, cell.measured.file, cell.measured.line)
		}
		out.WriteString("\n")
	}
	return out.String()
}
