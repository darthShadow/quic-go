# quic-go Patches

Base: quic-go `v0.64.1`, commit `ed90516797ec8ed7314c2b163797eca58fd5a2b8`.
Module path: `github.com/quic-go/quic-go`.

## Patch Inventory

| Patch | Intent and Opt-In | Tests | Source |
|---|---|---|---|
| Q1 | `Config.CongestionControl = CongestionControlBBRv3` selects the BBRv3 port for initial and migrated paths. The zero value keeps upstream's Reno-mode cubicSender. `CongestionControlName` uses the same factory. | `TestDefaultConfigSelectsUpstreamSender`, `TestSenderIdentityBBRv3`, `TestCongestionControlName`. | `bbr3-quic-go.patch`. |
| Q2 | `BBRNoLowerBounds` and `BBRLossThresholdPercent` belong to each sender. Zero percent selects 2 percent; 1-100 are accepted. | `TestBBRLossThresholdBounds`, `TestCongestionControlConfigValidation`. | `bbr3-quic-go.patch`. |
| Q3 | `PMTUBlackHoleDetection` enables post-handshake data-packet EMSGSIZE and two-PTO fallback to 1200 bytes, then re-probing. Probe errors remain probe losses. | `TestPMTUErrorSignal`. | `quic-pmtu-blackhole.patch`. |
| Q4 | Adapter DF/GSO capabilities opt in through methods. `ScaledStreamGapLimit` enables the current-window gap bound and chooses the search end by offset. Default streams and CRYPTO retain 1000 gaps. The offset rule does not bound the search: a peer that places a large front gap and a dense gap cluster near the tail still costs about window/1024 gap visits per frame (49,153 at 48 MiB, against upstream's 1,000). Enable it only on receivers whose peers are trusted; keep the server's receive side on the upstream limit. | `TestAdapterCapabilities`, `TestAdapterGSOSegments`. | `quic-adapter-pmtu.patch`. |
| Q5 | `shunpike_research` compiles the QG controls, diagnostic output, `BenchSnapshot`, `BenchBBRTuning` and their counters. They are absent from default builds. | Research-tag control checks and both tagged/default builds. | `quic-bbr-tuning.patch`, `quic-bbr-defaults.patch` and Q4 counter hunks. |
| Q6 | `Config.ACKDecimation`, off by default, controls ACKs sent by either endpoint for application data. After 100 received packets it ACKs every 4 ack-eliciting packets with delay min(25 ms, min RTT / 4). No environment switch. | `TestConfigACKDecimation`, `TestACKDecimationConnection`, `TestACKDecimationCadence`, `TestACKDecimationCountsAllPackets`, `TestACKDecimationDelay`. | `quic-pacer-ack4.patch`. |
| Q7 | BBRv3 uses the measured quiche pacer: initial and idle bursts, 0.5 ms lumps bounded by one GSO buffer and a quarter of cwnd, late-wakeup catch-up with a 4 ms debt cap. Only ack-eliciting packets admitted with at least one MSS of pacing budget advance the schedule; packets sent without that budget (PING-ACKs, PTO probes, coalesced extras) do not add debt. The default sender retains the upstream token bucket. | `TestQuichePacerBursts`, `TestQuichePacerLumps`, `TestBBR3LumpSpansHalfMillisecond`, `TestQuichePacerCatchup`, `TestQuichePacerDebtCap`, `TestQuichePacerResets`, `TestBBR3PacerChargesOnlyAdmittedPackets`, `TestBBR3PacerChargesAdmittedPackets`; upstream pacer tests. | `quic-pacer-ack4.patch`. |
| Q8 | Probe now: `(*Conn).ProbeNow()` requests up to two PING-only 1-RTT packets in PTO backoff with outstanding data. Calls merge and bypass the congestion window while the send queue has room. The call preserves outstanding frames, the scheduled timeout, the probe count, congestion and RTT state; without a call, upstream behavior stays unchanged. | `TestProbeNow`, `TestProbeNowAfterBackoff`. | Direct path recovery. |

`CongestionControlBBRv3` is a package constant used in `Config.CongestionControl`.
BBRv3 does not respond to CE marks, so selecting it disables ECN.
Defaults in normal and research builds are 10 s ProbeRTT,
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
`internal/congestion/anchor_test.go` records the anchor parameters, with
Bbr2Sender and Bbr3Sender values beside fork values. Gains use Q8 quantization and
window sizes use a common 1200-byte MSS. Structural rows exercise sender behavior;
`TestAnchorDefaults` checks shipped values. Deviations and their reasons follow.

## Deviations

**D1.** The explicit loss profile disables lower bounds and selects 5 percent
loss; default BBR keeps active lower bounds and 2 percent. The explicit profile
improves random-loss goodput; congestion loss and competition remain unmeasured.

**D2.** ProbeRTT can start from Startup instead of only at the end of ProbeDown;
both intervals are 10 s. This behavior is retained from the measured defaults
bundle; its individual effect is not isolated.

**D4.** ACK aggregation uses one Startup round instead of Bbr2Sender's ten; later
rounds are ten and uncapped. This behavior is retained from the measured defaults
bundle; its individual effect is not isolated.

**D6.** Loss tests run per packet; ProbeUp caps inflight_hi and uses a bandwidth
plateau exit, with a 2.25 cwnd gain. Combined exit changes increase latency; the
anchor ProbeUp variant raises drops without a robust throughput gain. The cwnd
gain's individual effect is not isolated.

**D7.** ProbeDown exits only when drained instead of also after one min_rtt.
Alternative exit rules increase latency with deep queues; the ProbeDown exit
effect is not isolated.

**D8.** Startup pacing uses Q8 gain 710 and exits after six loss events; Drain
uses gain 128 and a three-round limit. The port retains these values without
measurements.

**D9.** Two consecutive SendAny send opportunities with empty packers explicitly
mark delivery samples app-limited through max(delivered + inflight, 1), with no
application Write started on any stream between them. At each pass, every send
stream must have no queued data or pending Write and must have stream flow-control
credit, and the connection must have flow-control credit. A Write that is blocked
or still waking after its last frame is packed does not trigger the signal.
Stream and connection flow-control limits, cwnd, pacing, send-queue, and
amplification limits do not trigger it. An application that drains a chunk and
goes idle still triggers it on the second qualifying pass. The zero-inflight
heuristic stays beside this signal. The measured bundle shows no post-Startup
stall; the app-limited change's individual effect is not isolated.

**D10.** The maximum cwnd is 50,400,000 bytes (42,000 packets at 1,200 bytes)
instead of the anchor's 2,000 packets. The shared constant also sets the default
CUBIC sender's ceiling to 42,000 packets, scaled by the current datagram size.
The larger ceiling supports high-bandwidth paths with long RTTs; doubling it
reduces measured goodput at 350 ms RTT. Server memory per connection remains
unmeasured.

**D11.** BBRv3 charges its quiche pacer only for ack-eliciting packets admitted
with at least one MSS of pacing budget. Unadmitted packets leave its schedule
unchanged. Burst tokens, lumps, idle restart, and the 4 ms catch-up cap stay in
place. The measured bundle shows no post-Startup stall; the pacer admission
change's individual effect is not isolated.

Lumps span 0.5 ms at the pacing rate instead of the anchor's two packets.
They have a two-packet minimum, bounded by one GSO buffer and a quarter of cwnd;
a small window, bandwidth below 1.2 Mbit/s, or a full cwnd reduces them to one
packet. Larger lumps reduce send calls and CPU cost at high rates while keeping
measured goodput and latency within the baseline intervals.

## Upstream Test Fixture

`configWithNonZeroNonFunctionFields` in `config_test.go` supplies nonzero values
for the five new Config fields. The Clone and populateConfig checks cover them.
