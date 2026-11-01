# Silo Fork Changes

Base: quic-go `v0.64.1`, commit `ed90516797ec8ed7314c2b163797eca58fd5a2b8`.
Module path: `github.com/quic-go/quic-go`.

## Patch Inventory

| Patch | Intent and Opt-In | Tests | Source |
|---|---|---|---|
| Q1 | `Config.CongestionControl = CongestionControlBBRv3` selects the BBRv3 port for initial and migrated paths. The zero value keeps upstream's Reno-mode cubicSender. `CongestionControlName` uses the same factory. | A1, A2, A4; `TestCongestionControlName`. | `bbr3-quic-go.patch`. |
| Q2 | `BBRNoLowerBounds` and `BBRLossThresholdPercent` belong to each sender. Zero percent selects 2 percent; 1-100 are accepted. | A5; `TestBBRLossThresholdBounds`, `TestSiloConfigValidation`. | `bbr3-quic-go.patch`. |
| Q3 | `PMTUBlackHoleDetection` enables post-handshake data-packet EMSGSIZE and two-PTO fallback to 1200 bytes, then re-probing. Probe errors remain probe losses. | A6; `TestPMTUErrorSignal`. | `quic-pmtu-blackhole.patch`. |
| Q4 | Adapter DF/GSO capabilities opt in through methods. `ScaledStreamGapLimit` enables the current-window gap bound and chooses the search end by offset. Default streams and CRYPTO retain 1000 gaps. The offset rule does not bound the search: a peer that places a large front gap and a dense gap cluster near the tail still costs about window/1024 gap visits per frame (49,153 at 48 MiB, against upstream's 1,000). Enable it only on receivers whose peers are trusted; keep the server's receive side on the upstream limit. | A8; `TestAdapterCapabilities`, `TestAdapterGSOSegments`. | `quic-adapter-pmtu.patch`. |
| Q5 | `shunpike_research` compiles the QG controls, diagnostic output, `BenchSnapshot`, `BenchBBRTuning` and their counters. They are absent from default builds. | Research-tag control checks and both tagged/default builds. | `quic-bbr-tuning.patch`, `quic-bbr-defaults.patch` and Q4 counter hunks. |
| Q6 | `Config.ACKDecimation`, off by default, controls ACKs sent by either endpoint for application data. After 100 received packets it ACKs every 4 ack-eliciting packets with delay min(25 ms, min RTT / 4). No environment switch. | `TestConfigACKDecimation`, `TestSiloACKDecimationConnection`, `TestACKDecimationCadence`, `TestACKDecimationCountsAllPackets`, `TestACKDecimationDelay`. | `quic-pacer-ack4.patch` receiver policy; R56 config wiring. |
| Q7 | BBRv3 uses the measured quiche pacer: initial and idle bursts, two-packet lumps, late-wakeup catch-up with a 4 ms debt cap. Only ack-eliciting packets admitted with at least one MSS of pacing budget advance the schedule; packets sent without that budget (PING-ACKs, PTO probes, coalesced extras) do not add debt (R140 at `ea20f93`: no post-Startup stall; pacer effect not isolated). The default sender retains the upstream token bucket. | `TestQuichePacerBursts`, `TestQuichePacerLumps`, `TestQuichePacerCatchup`, `TestQuichePacerDebtCap`, `TestQuichePacerResets`, `TestBBR3PacerChargesOnlyAdmittedPackets`, `TestBBR3PacerChargesAdmittedPackets`; upstream pacer tests. | `quic-pacer-ack4.patch`; R55 promotion and R56 placement. |

`CongestionControlBBRv3` is a package constant used in `Config.CongestionControl`.
BBRv3 does not respond to CE marks, so selecting it disables ECN.
C2 defaults in normal and research builds are 10 s ProbeRTT,
0 percent pacing margin, uncapped ACK aggregation over ten rounds, and 15 percent
headroom. The research control `QG_BBR_DEFAULTS=previous` keeps the prior profile available for experiments.
Individual research controls override the selected research profile.

## Adapter Contract

The adapter retains socket ownership and logical addresses. `SupportsDF() bool`
reports Don't Fragment support. GSO uses `SupportsGSO() bool` and
`WriteSegments([]byte, net.Addr, uint16) (int, error)`; zero segment size requests
one datagram. Write errors propagate. A plain packet connection keeps upstream
behavior.

## Quiche Anchor

Google quiche pin: `7980e3d5d89a389035793924f96682e94c867860`.
`internal/congestion/anchor_test.go` carries the 25 retained parameter rows from the
anchor table, with Bbr2Sender and Bbr3Sender values beside fork values.
Gains use Q8 quantization and window sizes use a common 1200-byte MSS. Structural
rows exercise sender behavior. The Go table holds each value and typed reason;
`TestAnchorDefaults` checks shipped values. The typed research list beside it
records measured alternatives and renders the Research Arms section below.
Deviations are records, not runtime switches.

## Deviations

Values and typed reasons are in `internal/congestion/anchor_test.go`.
Reasons distinguish measured bundles and alternatives from individual effects
that are not isolated or remain unmeasured.

| ID | Difference | Reason |
|---|---|---|
| D1 | The explicit loss profile disables lower bounds and selects 5 percent loss; default BBR keeps active lower bounds and 2 percent. | Random-loss goodput increases in `measurements/n4-results.md:1`; congestion loss and competition remain unmeasured. |
| D2 | ProbeRTT can start from Startup instead of only at the end of ProbeDown; both intervals are 10 s. | Measured only inside C2 (W22j); the individual effect is not isolated. |
| D4 | ACK aggregation uses one Startup round instead of Bbr2Sender's ten; later rounds are ten and uncapped. | Measured only inside C2 (W22j); the individual effect is not isolated. |
| D6 | Loss tests run per packet; ProbeUp caps inflight_hi and uses a bandwidth plateau exit, with a 2.25 cwnd gain. | W29a ProbeBW Exits fails the ping guard (`measurements/w29a-confirm.md:28-31`); ProbeUp Anchor has no robust police-100 gain and raises drops (`measurements/w29a-confirm.md:32-33`). The 2.25 cwnd gain is not isolated. |
| D7 | ProbeDown exits only when drained instead of also after one min_rtt. | W29a ProbeBW Exits fails the ping guard at fifo-deep-50/100 (`measurements/w29a-confirm.md:29-30`); the ProbeDown exit effect is not isolated. |
| D8 | Startup pacing uses Q8 gain 710 and exits after six loss events; Drain uses gain 128 and a three-round limit. | Unmeasured. |
| D9 | Two consecutive SendAny send opportunities with empty packers explicitly mark delivery samples app-limited through max(delivered + inflight, 1), with no application Write started on any stream between them. At each pass, every send stream must have no queued data or pending Write and must have stream flow-control credit, and the connection must have flow-control credit. A Write that is blocked or still waking after its last frame is packed does not trigger the signal. Stream and connection flow-control limits, cwnd, pacing, send-queue, and amplification limits do not trigger it. An application that drains a chunk and goes idle still triggers it on the second qualifying pass. The zero-inflight heuristic stays beside this signal. | R140 at `ea20f93` records no post-Startup stall; the app-limited change is not isolated. |
| D10 | The maximum cwnd is 50,400,000 bytes (42,000 packets at 1,200 bytes) instead of the anchor's 2,000 packets. The shared constant also sets the default CUBIC sender's ceiling to 42,000 packets, scaled by the current datagram size. | R140 at `ea20f93`: all 2,860 cap samples read 50,400,000 bytes; steady goodput at 1000 Mbit/s and 0.1% loss is 446-930 Mbit/s across 110-350 ms RTTs. R173 finds doubling the cap hurts at 350 ms (885 -> 745 Mbit/s in the 0.1% loss, 128 MiB cell). R156/R162 measure client RSS; server memory per connection is unmeasured. |
| D11 | BBRv3 charges its quiche pacer only for ack-eliciting packets admitted with at least one MSS of pacing budget. Unadmitted packets leave its schedule unchanged. Burst tokens, lumps, idle restart, and the 4 ms catch-up cap stay in place. | R140 at `ea20f93` records no post-Startup stall; the pacer admission change is not isolated. |

## Research Arms

Generated from `anchorResearchArms` in `internal/congestion/anchor_test.go`.
W29a, W42a and W42b are n=10 confirms; W37a and W43a are n=5 screens. W51a confirms
Pacer + ACK4 with n=30 in deep-FIFO cells and n=10 elsewhere; R55 promotes it and R56 sets
the shipped config and pacer placement. Measurement IDs keep the phases separate. Ratios compare with
`n4-c2`, not Google quiche. Anchor sources below use quiche `7980e3d5d89a`,
under `quiche/quic/core/`. Unmeasured costs do not establish a guard pass.

The baseline is C2 with the explicit Silo loss profile: lower bounds off,
5 percent loss threshold. The default sender retains its own loss policy.

Costs show baseline -> arm. CPU is process seconds per decimal GB in the
application measurement window. Server+client is the sum of the two reported
medians, not the median of per-run sums. Drops include configured random loss
in WAN cells. W29a and W37a omit competition shares and WAN household ping;
W42a, W42b and W43a measure both in every cell with a competing CUBIC download. Their share
guard uses N4 bytes / (N4 bytes + CUBIC bytes); both shares appear below.
Each row cites its evidence snapshot under `measurements/`.

### Pacer Catch-Up

- Overlay: `quic-pacer-catchup.patch`.
- Anchor: ideal send schedule; catch up while pacing-limited; 1 ms allowance; 10-packet initial burst and up to 2-packet lumps (`congestion_control/pacing_sender.cc:66-117,146-156`).
- Experimental value: same schedule and allowance; catch-up debt capped at 4 ms; burst budget max(5 ms of rate, 2 MSS).
- Shipped value: BBRv3 uses the measured 4 ms quiche pacer through the Pacer + ACK4 promotion; this standalone arm retains its earlier rejection; the default sender keeps the upstream token bucket.
- Status: **not promoted (#456): competitor share fails fifo-deep-50/100; goodput bound fails fifo-deep-25; server CPU fails wan-200**. W29a records solo confirms; W42a adds competing CUBIC and household ping in every cell. At wan-200, both server and server+client CPU fail the guard. W42a evaluates the competitor-share guard on N4 share.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 1.023435 [1.022883, 1.026914] | 94.750 -> 93.200 | unmeasured | unmeasured | 23.095347 -> 25.251741 | 58.952741 -> 57.511123 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:21` |
| fifo-deep-50 | 1.035729 [1.032515, 1.041484] | 98.250 -> 88.750 | unmeasured | unmeasured | 22.123442 -> 23.089075 | 53.990989 -> 52.490880 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:22` |
| fifo-deep-100 | 1.041747 [1.037661, 1.045183] | 98.450 -> 87.500 | unmeasured | unmeasured | 18.977052 -> 20.781495 | 46.452318 -> 45.903755 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:23` |
| wan-200 | 1.040649 [1.038319, 1.044593] | unmeasured | unmeasured | unmeasured | 17.521959 -> 19.721513 | 37.112749 -> 39.469894 | 4943.0 -> 5129.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:24` |
| police-25 | 1.024194 [1.017411, 1.031669] | 40.000 -> 40.000 | unmeasured | unmeasured | 25.438197 -> 24.549573 | 48.163202 -> 45.871201 | 285.0 -> 458.5 | W29a/confirm-police; `measurements/w29a-confirm.md:25` |
| police-100 | 1.125387 [1.110590, 1.139633] | 40.000 -> 40.000 | unmeasured | unmeasured | 10.184706 -> 10.443545 | 19.491200 -> 19.805654 | 661.0 -> 585.5 | W29a/confirm-police; `measurements/w29a-confirm.md:26` |
| police-50 | 1.069945 [1.063795, 1.081274] | 40.000 -> 40.000 | unmeasured | unmeasured | 15.154407 -> 15.337595 | 29.117327 -> 28.899171 | 369.0 -> 438.0 | W29a/confirm-police50; `measurements/w29a-confirm.md:27` |
| fifo-deep-25 | 0.955057 [0.897065, 0.994486] | 339.000 -> 339.000 | 0.579734 -> 0.555846 | 0.420266 -> 0.444154 | 30.182815 -> 31.391419 | 66.511510 -> 66.501995 | 428.5 -> 386.5 | W42a/guards-n10; `measurements/w42a-guards.md:21` |
| fifo-deep-50 | 1.298443 [1.210310, 1.342627] | 339.000 -> 340.000 | 0.471315 -> 0.612535 | 0.528685 -> 0.387465 | 25.418496 -> 25.640900 | 60.277774 -> 56.992322 | 501.0 -> 823.0 | W42a/guards-n10; `measurements/w42a-guards.md:22` |
| fifo-deep-100 | 1.250345 [1.170691, 1.425831] | 336.500 -> 339.000 | 0.473149 -> 0.593252 | 0.526851 -> 0.406748 | 21.569459 -> 22.103210 | 52.269318 -> 49.961223 | 514.5 -> 921.5 | W42a/guards-n10; `measurements/w42a-guards.md:23` |
| wan-200 | 1.045751 [1.042473, 1.048549] | 71.500 -> 71.150 | 0.983912 -> 0.985264 | 0.016088 -> 0.014736 | 17.719177 -> 20.078733 | 37.522565 -> 40.234854 | 4943.5 -> 5211.0 | W42a/guards-n10; `measurements/w42a-guards.md:24` |
| police-25 | 1.016009 [0.995494, 1.023520] | 40.000 -> 40.000 | 0.902290 -> 0.891895 | 0.097710 -> 0.108105 | 24.748968 -> 25.129568 | 46.681869 -> 46.765930 | 558.5 -> 456.5 | W42a/guards-n10; `measurements/w42a-guards.md:25` |
| police-50 | 1.058015 [1.039034, 1.087516] | 40.000 -> 40.000 | 0.873623 -> 0.873577 | 0.126377 -> 0.126423 | 15.631970 -> 15.921877 | 30.022077 -> 29.938624 | 619.0 -> 558.5 | W42a/guards-n10; `measurements/w42a-guards.md:26` |
| police-100 | 1.100264 [1.080575, 1.121217] | 40.000 -> 40.000 | 0.880408 -> 0.893180 | 0.119592 -> 0.106820 | 10.566045 -> 10.738549 | 20.396907 -> 20.454566 | 950.5 -> 792.5 | W42a/guards-n10; `measurements/w42a-guards.md:27` |

### ProbeBW Exits

- Overlay: `quic-probebw-exits.patch`.
- Anchor: ProbeUp: after one round, inflight >= 1.25 BDP + 2 MSS + max ACK height; ProbeDown: one min_rtt or drained; ProbeRTT: only after ProbeDown, 200 ms at 0.5 BDP, no extra round (`congestion_control/bbr2_probe_bw.cc:52-65,129-132,255-263,429-468; congestion_control/bbr2_probe_rtt.cc:26-41`).
- Experimental value: anchor queue and phase exits, including deferred ProbeRTT entry and no extra round.
- Shipped value: n4-c2 keeps three-round bandwidth plateau ProbeUp exit, drained-only ProbeDown exit, ProbeRTT entry from any state and 200 ms plus one round.
- Status: **not promoted: ping guard**. Household ping p95 rises above +5% at fifo-deep-50/100. Competitor share is unmeasured.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 1.026745 [1.026646, 1.029087] | 94.750 -> 94.000 | unmeasured | unmeasured | 23.095347 -> 23.609290 | 58.952741 -> 55.973380 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:28` |
| fifo-deep-50 | 1.037358 [1.033706, 1.042076] | 98.250 -> 120.500 | unmeasured | unmeasured | 22.123442 -> 21.256780 | 53.990989 -> 52.593586 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:29` |
| fifo-deep-100 | 1.031059 [1.029104, 1.033862] | 98.450 -> 119.000 | unmeasured | unmeasured | 18.977052 -> 18.655319 | 46.452318 -> 45.815044 | 0.0 -> 0.0 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:30` |
| wan-200 | 1.016036 [1.013288, 1.017714] | unmeasured | unmeasured | unmeasured | 17.521959 -> 17.427825 | 37.112749 -> 36.940734 | 4943.0 -> 5046.5 | W29a/confirm-deep-wan; `measurements/w29a-confirm.md:31` |

### ProbeUp Anchor

- Overlay: `quic-probeup-anchor.patch`.
- Anchor: ProbeUp ignores inflight_hi and slope growth; per-ACK round loss test: >=2 loss events and >2% of latest ACKed packet send-state inflight; inflight_hi = max(that inflight, 0.7 x target) (`congestion_control/bbr2_probe_bw.cc:76-87,172-218,321-327; congestion_control/bbr2_misc.cc:163-166,323-328,354-386,394-396`).
- Experimental value: anchor ProbeUp cap, loss test and send-state inflight_hi; Startup stays unchanged.
- Shipped value: n4-c2 keeps ProbeUp inflight_hi cap and slope growth, per-lost-packet loss test and lost-prefix inflight estimate.
- Status: **rejected: no robust gain at police-100; drop cost**. Police-25 gains with a lower 95% bound above 1, but drops rise from 285 to 1029 (3.61x). Police-100 has no robust gain. Competitor share is unmeasured.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| police-25 | 1.013829 [1.003003, 1.023060] | 40.000 -> 40.000 | unmeasured | unmeasured | 25.438197 -> 22.744889 | 48.163202 -> 43.508086 | 285.0 -> 1029.0 | W29a/confirm-police; `measurements/w29a-confirm.md:32` |
| police-100 | 1.008645 [0.991490, 1.029429] | 40.000 -> 40.000 | unmeasured | unmeasured | 10.184706 -> 10.693970 | 19.491200 -> 21.017458 | 661.0 -> 1636.0 | W29a/confirm-police; `measurements/w29a-confirm.md:33` |

### Pacer + ACK4

- Overlay: `quic-pacer-ack4.patch`.
- Anchor: receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance (`quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156`).
- Experimental value: client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 4 ms, burst budget max(5 ms of rate, 2 MSS).
- Shipped value: Config.ACKDecimation opts either endpoint into the measured receiver policy, off by default; BBRv3 always uses the measured quiche pacer with 4 ms catch-up debt, initial and idle bursts, two-packet lumps and max(5 ms of rate, 2 MSS) budget; the default sender retains the upstream token bucket.
- Status: **promoted by user decision R55 after W51a confirm; share and household-ping trade-offs accepted**. W51a confirms n4-pacer-ack4 against n4-c2 with n=30 in each deep-FIFO cell and n=10 elsewhere. R55 (Promote anyway) accepts N4-share ratios 1.229 at fifo-deep-50 and 1.291 at fifo-deep-100 against a competing CUBIC download, and household ping ratio 1.21 at wan-1000. These ratios are the approved promotion record; the full W51a per-cell output is not present in this checkout. R56 selects one receiver config field and the pacer in BBRv3. W37a, W42b and W43a remain the earlier screen and confirm records below; their failed guards are not erased.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 0.990398 [0.907104, 1.026421] | 339.000 -> 339.000 | 0.575675 -> 0.573552 | 0.424325 -> 0.426448 | 31.582912 -> 27.511707 | 69.127514 -> 58.565609 | 372.0 -> 319.0 | W42b/confirm-n10; `measurements/w42b-confirm.md:20` |
| fifo-deep-50 | 1.206904 [1.127228, 1.305721] | 338.500 -> 339.000 | 0.470552 -> 0.569898 | 0.529448 -> 0.430102 | 26.249233 -> 19.161096 | 61.974513 -> 47.460461 | 459.5 -> 854.5 | W42b/confirm-n10; `measurements/w42b-confirm.md:21` |
| fifo-deep-100 | 1.247453 [1.123465, 1.303705] | 337.500 -> 340.000 | 0.491925 -> 0.614166 | 0.508075 -> 0.385834 | 21.902010 -> 14.689101 | 52.910606 -> 39.505462 | 608.0 -> 1119.0 | W42b/confirm-n10; `measurements/w42b-confirm.md:22` |
| wan-200 | 1.042150 [1.039890, 1.045156] | 70.550 -> 70.150 | 0.985387 -> 0.985610 | 0.014613 -> 0.014390 | 18.099489 -> 13.255145 | 38.227231 -> 30.511240 | 4956.0 -> 5220.5 | W42b/confirm-n10; `measurements/w42b-confirm.md:23` |
| police-25 | 1.020605 [1.001578, 1.042997] | 40.000 -> 40.000 | 0.886006 -> 0.888313 | 0.113994 -> 0.111687 | 25.276819 -> 23.328736 | 47.909583 -> 42.643490 | 502.5 -> 447.5 | W42b/confirm-n10; `measurements/w42b-confirm.md:24` |
| police-50 | 1.068936 [1.042979, 1.084949] | 40.000 -> 40.000 | 0.881161 -> 0.887582 | 0.118839 -> 0.112418 | 15.723231 -> 15.418482 | 30.249596 -> 28.868793 | 621.5 -> 591.0 | W42b/confirm-n10; `measurements/w42b-confirm.md:25` |
| police-100 | 1.105925 [1.068615, 1.123288] | 40.000 -> 40.000 | 0.877400 -> 0.889122 | 0.122600 -> 0.110878 | 10.801199 -> 10.120728 | 20.873124 -> 19.259237 | 868.0 -> 805.5 | W42b/confirm-n10; `measurements/w42b-confirm.md:26` |
| wan-1000 | 1.146500 [1.117554, 1.161462] | 59.650 -> 65.250 | 0.996471 -> 0.997563 | 0.003529 -> 0.002437 | 13.063060 -> 13.014796 | 25.648232 -> 23.882669 | 27059.5 -> 31698.0 | W42b/confirm-n10; `measurements/w42b-confirm.md:27` |
| fifo-deep-25 | 1.026755 [1.024970, 1.028601] | 95.800 -> 94.800 | unmeasured | unmeasured | 22.262715 -> 20.204617 | 56.776789 -> 47.493014 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:19` |
| police-25 | 1.028656 [1.023318, 1.035284] | 40.000 -> 40.000 | unmeasured | unmeasured | 24.416950 -> 21.434651 | 46.113376 -> 39.316611 | 350.0 -> 423.0 | W37a/screen-n5; `measurements/w37a-screen.md:20` |
| fifo-deep-50 | 1.041236 [1.032997, 1.044852] | 99.100 -> 91.000 | unmeasured | unmeasured | 21.184513 -> 13.858407 | 52.082059 -> 38.307889 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:21` |
| police-50 | 1.058261 [1.054998, 1.075041] | 40.000 -> 40.000 | unmeasured | unmeasured | 14.704604 -> 13.934854 | 28.160766 -> 26.298258 | 378.0 -> 405.0 | W37a/screen-n5; `measurements/w37a-screen.md:22` |
| fifo-deep-100 | 1.042594 [1.037195, 1.045236] | 98.500 -> 86.800 | unmeasured | unmeasured | 18.459705 -> 12.752245 | 45.140035 -> 34.022187 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:23` |
| police-100 | 1.135807 [1.113026, 1.147332] | 40.000 -> 40.000 | unmeasured | unmeasured | 9.805493 -> 9.178903 | 19.015446 -> 17.467665 | 602.0 -> 576.0 | W37a/screen-n5; `measurements/w37a-screen.md:24` |
| wan-200 | 1.040107 [1.038474, 1.045036] | unmeasured | unmeasured | unmeasured | 16.916285 -> 12.092624 | 35.871049 -> 28.283490 | 5042.0 -> 5127.0 | W37a/screen-n5; `measurements/w37a-screen.md:25` |
| wan-1000 | 1.119103 [1.116824, 1.124973] | unmeasured | unmeasured | unmeasured | 12.236751 -> 11.841269 | 24.094476 -> 22.101433 | 26996.0 -> 31572.0 | W37a/screen-n5; `measurements/w37a-screen.md:26` |
| fifo-deep-25 | 0.937126 [0.718495, 1.111353] | 339.000 -> 339.000 | 0.598890 -> 0.562224 | 0.401110 -> 0.437776 | 29.266039 -> 27.425970 | 65.413391 -> 57.613559 | 397.0 -> 260.0 | W43a/screen-n5; `measurements/w43a-screen.md:21` |
| fifo-deep-50 | 1.243507 [1.077960, 1.308198] | 339.000 -> 339.000 | 0.466602 -> 0.580636 | 0.533398 -> 0.419364 | 25.378138 -> 18.996637 | 60.135386 -> 46.283166 | 510.0 -> 755.0 | W43a/screen-n5; `measurements/w43a-screen.md:22` |
| fifo-deep-100 | 1.415492 [1.116215, 2.773127] | 335.000 -> 339.000 | 0.484823 -> 0.694424 | 0.515177 -> 0.305576 | 21.408615 -> 14.243198 | 51.777205 -> 38.421292 | 352.0 -> 1444.0 | W43a/screen-n5; `measurements/w43a-screen.md:23` |
| wan-200 | 1.045057 [1.038992, 1.049289] | 70.500 -> 72.200 | 0.985755 -> 0.986146 | 0.014245 -> 0.013854 | 17.606127 -> 12.883846 | 37.233723 -> 29.752854 | 4991.0 -> 5241.0 | W43a/screen-n5; `measurements/w43a-screen.md:24` |
| police-25 | 0.994744 [0.964853, 1.028487] | 40.000 -> 40.000 | 0.900413 -> 0.876384 | 0.099587 -> 0.123616 | 25.370763 -> 23.132160 | 47.728619 -> 42.062844 | 562.0 -> 440.0 | W43a/screen-n5; `measurements/w43a-screen.md:25` |
| police-50 | 1.068216 [1.041370, 1.099487] | 40.000 -> 40.000 | 0.877078 -> 0.886249 | 0.122922 -> 0.113751 | 15.746558 -> 15.255887 | 30.036705 -> 28.408590 | 608.0 -> 598.0 | W43a/screen-n5; `measurements/w43a-screen.md:26` |
| police-100 | 1.104121 [1.058950, 1.158703] | 40.000 -> 40.000 | 0.871711 -> 0.884618 | 0.128289 -> 0.115382 | 10.411251 -> 9.740764 | 20.263628 -> 18.679836 | 856.0 -> 805.0 | W43a/screen-n5; `measurements/w43a-screen.md:27` |
| wan-1000 | 1.124292 [1.116135, 1.147177] | 61.300 -> 65.400 | 0.996527 -> 0.997878 | 0.003473 -> 0.002122 | 12.877602 -> 12.770190 | 25.269641 -> 23.504350 | 27028.0 -> 32314.0 | W43a/screen-n5; `measurements/w43a-screen.md:28` |

### Client ACK4

- Overlay: `quic-client-ack4.patch`.
- Anchor: receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4) (`quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303`).
- Experimental value: client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); sender pacing stays unchanged.
- Shipped value: Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 includes the promoted 4 ms quiche pacer, so the receiver-only experimental combination is not the shipped BBRv3 policy.
- Status: **rejected at screen: wan-1000 lower bound 0.918**. W37a screen only; no n=10 confirm. Competition shares and WAN household ping are unmeasured in this screen.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 0.976643 [0.975452, 0.987104] | 95.800 -> 94.400 | unmeasured | unmeasured | 22.262715 -> 21.813468 | 56.776789 -> 52.680431 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:27` |
| police-25 | 1.029372 [1.023394, 1.035170] | 40.000 -> 40.000 | unmeasured | unmeasured | 24.416950 -> 21.103266 | 46.113376 -> 38.540072 | 350.0 -> 384.0 | W37a/screen-n5; `measurements/w37a-screen.md:28` |
| fifo-deep-50 | 0.972198 [0.964505, 0.979905] | 99.100 -> 97.700 | unmeasured | unmeasured | 21.184513 -> 12.801937 | 52.082059 -> 40.120787 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:29` |
| police-50 | 0.976837 [0.964015, 1.000765] | 40.000 -> 40.000 | unmeasured | unmeasured | 14.704604 -> 14.200725 | 28.160766 -> 27.318418 | 378.0 -> 361.0 | W37a/screen-n5; `measurements/w37a-screen.md:30` |
| fifo-deep-100 | 0.964726 [0.960625, 0.972009] | 98.500 -> 98.700 | unmeasured | unmeasured | 18.459705 -> 12.104328 | 45.140035 -> 36.009877 | 0.0 -> 0.0 | W37a/screen-n5; `measurements/w37a-screen.md:31` |
| police-100 | 1.004591 [0.979370, 1.014070] | 40.000 -> 40.000 | unmeasured | unmeasured | 9.805493 -> 9.111460 | 19.015446 -> 17.683228 | 602.0 -> 604.0 | W37a/screen-n5; `measurements/w37a-screen.md:32` |
| wan-200 | 0.976616 [0.975007, 0.982565] | unmeasured | unmeasured | unmeasured | 16.916285 -> 11.058658 | 35.871049 -> 27.538582 | 5042.0 -> 4833.0 | W37a/screen-n5; `measurements/w37a-screen.md:33` |
| wan-1000 | 0.976989 [0.918043, 0.999142] | unmeasured | unmeasured | unmeasured | 12.236751 -> 9.519254 | 24.094476 -> 19.774047 | 26996.0 -> 25725.0 | W37a/screen-n5; `measurements/w37a-screen.md:34` |

### Pacer + ACK4 (1 ms Debt)

- Overlay: `quic-pacer-ack4-1ms.patch`.
- Anchor: receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance (`quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156`).
- Experimental value: client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 1 ms, burst budget max(2 ms of rate, 2 MSS).
- Shipped value: Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 uses the promoted 4 ms debt cap, not this 1 ms variant; the default sender retains the upstream token bucket.
- Status: **rejected (#456): competitor share fails fifo-deep-50/100; household ping fails wan-1000**. W43a n=5 screen only; also fails goodput and competitor share at fifo-deep-25. The competition guard uses N4 share.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 0.946716 [0.816483, 1.122725] | 339.000 -> 339.000 | 0.598890 -> 0.566913 | 0.401110 -> 0.433087 | 29.266039 -> 26.960931 | 65.413391 -> 57.418829 | 397.0 -> 419.0 | W43a/screen-n5; `measurements/w43a-screen.md:29` |
| fifo-deep-50 | 1.169637 [1.013924, 1.302959] | 339.000 -> 339.000 | 0.466602 -> 0.546008 | 0.533398 -> 0.453992 | 25.378138 -> 19.344821 | 60.135386 -> 46.731315 | 510.0 -> 656.0 | W43a/screen-n5; `measurements/w43a-screen.md:30` |
| fifo-deep-100 | 1.310977 [1.100796, 2.568369] | 335.000 -> 339.000 | 0.484823 -> 0.636245 | 0.515177 -> 0.363755 | 21.408615 -> 14.144870 | 51.777205 -> 38.343967 | 352.0 -> 1139.0 | W43a/screen-n5; `measurements/w43a-screen.md:31` |
| wan-200 | 1.043094 [1.037040, 1.048690] | 70.500 -> 70.100 | 0.985755 -> 0.985250 | 0.014245 -> 0.014750 | 17.606127 -> 12.757818 | 37.233723 -> 29.702861 | 4991.0 -> 5219.0 | W43a/screen-n5; `measurements/w43a-screen.md:32` |
| police-25 | 1.001112 [0.987054, 1.035070] | 40.000 -> 40.000 | 0.900413 -> 0.881543 | 0.099587 -> 0.118457 | 25.370763 -> 22.949545 | 47.728619 -> 41.829368 | 562.0 -> 453.0 | W43a/screen-n5; `measurements/w43a-screen.md:33` |
| police-50 | 1.062830 [1.042869, 1.093943] | 40.000 -> 40.000 | 0.877078 -> 0.882174 | 0.122922 -> 0.117826 | 15.746558 -> 15.294124 | 30.036705 -> 28.412174 | 608.0 -> 576.0 | W43a/screen-n5; `measurements/w43a-screen.md:34` |
| police-100 | 1.092273 [1.050162, 1.136564] | 40.000 -> 40.000 | 0.871711 -> 0.880386 | 0.128289 -> 0.119614 | 10.411251 -> 9.857504 | 20.263628 -> 18.795922 | 856.0 -> 777.0 | W43a/screen-n5; `measurements/w43a-screen.md:35` |
| wan-1000 | 1.121954 [1.114852, 1.144792] | 61.300 -> 67.100 | 0.996527 -> 0.997513 | 0.003473 -> 0.002487 | 12.877602 -> 12.878146 | 25.269641 -> 23.612652 | 27028.0 -> 31907.0 | W43a/screen-n5; `measurements/w43a-screen.md:36` |

### Pacer + ACK4 (No Debt)

- Overlay: `quic-pacer-ack4-nodebt.patch`.
- Anchor: receiver ACKs every 10 packets after the first 100; delay min(25 ms, min_rtt/4); ideal pacing schedule with late-wakeup catch-up and 1 ms allowance (`quic_received_packet_manager.cc:237-290,307-314,336-347; quic_constants.h:297-303; congestion_control/pacing_sender.cc:66-117,146-156`).
- Experimental value: client ACKs every 4 ack-eliciting packets after the first 100; delay min(25 ms, min_rtt/4); pacer catch-up debt capped at 0 ms, burst budget max(1 ms of rate, 2 MSS).
- Shipped value: Config.ACKDecimation opts either endpoint into ACK4, off by default; BBRv3 uses the promoted 4 ms debt cap, not this no-debt variant; the default sender retains the upstream token bucket.
- Status: **rejected (#456): competitor share fails fifo-deep-50/100; household ping fails wan-1000**. W43a n=5 screen only; also fails goodput and competitor share at fifo-deep-25. The police-25 goodput bound also fails. The competition guard uses N4 share.

| Cell | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Measurement ID and Evidence |
|---|---|---|---|---|---|---|---|---|
| fifo-deep-25 | 0.932289 [0.739928, 1.105616] | 339.000 -> 339.000 | 0.598890 -> 0.558374 | 0.401110 -> 0.441626 | 29.266039 -> 26.882148 | 65.413391 -> 57.261382 | 397.0 -> 393.0 | W43a/screen-n5; `measurements/w43a-screen.md:37` |
| fifo-deep-50 | 1.301169 [1.127946, 1.385429] | 339.000 -> 339.000 | 0.466602 -> 0.607618 | 0.533398 -> 0.392382 | 25.378138 -> 18.373205 | 60.135386 -> 45.628884 | 510.0 -> 975.0 | W43a/screen-n5; `measurements/w43a-screen.md:38` |
| fifo-deep-100 | 1.334382 [1.248103, 2.614223] | 335.000 -> 339.000 | 0.484823 -> 0.654445 | 0.515177 -> 0.345555 | 21.408615 -> 14.010130 | 51.777205 -> 38.022232 | 352.0 -> 1144.0 | W43a/screen-n5; `measurements/w43a-screen.md:39` |
| wan-200 | 1.044034 [1.037974, 1.048754] | 70.500 -> 71.700 | 0.985755 -> 0.985609 | 0.014245 -> 0.014391 | 17.606127 -> 12.728400 | 37.233723 -> 29.735430 | 4991.0 -> 5237.0 | W43a/screen-n5; `measurements/w43a-screen.md:40` |
| police-25 | 0.996116 [0.933622, 1.029905] | 40.000 -> 40.000 | 0.900413 -> 0.878738 | 0.099587 -> 0.121262 | 25.370763 -> 22.899806 | 47.728619 -> 41.649489 | 562.0 -> 477.0 | W43a/screen-n5; `measurements/w43a-screen.md:41` |
| police-50 | 1.053136 [1.033365, 1.083965] | 40.000 -> 40.000 | 0.877078 -> 0.871767 | 0.122922 -> 0.128233 | 15.746558 -> 15.148350 | 30.036705 -> 28.322629 | 608.0 -> 573.0 | W43a/screen-n5; `measurements/w43a-screen.md:42` |
| police-100 | 1.123063 [1.063637, 1.163394] | 40.000 -> 40.000 | 0.871711 -> 0.900381 | 0.128289 -> 0.099619 | 10.411251 -> 9.820783 | 20.263628 -> 18.675227 | 856.0 -> 867.0 | W43a/screen-n5; `measurements/w43a-screen.md:43` |
| wan-1000 | 1.123688 [1.117187, 1.146561] | 61.300 -> 64.400 | 0.996527 -> 0.997689 | 0.003473 -> 0.002311 | 12.877602 -> 12.867617 | 25.269641 -> 23.613026 | 27028.0 -> 31471.0 | W43a/screen-n5; `measurements/w43a-screen.md:44` |

## Upstream Test Fixture

`configWithNonZeroNonFunctionFields` in `config_test.go` supplies nonzero values
for the five new Config fields. The Clone and populateConfig checks cover them.
