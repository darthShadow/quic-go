# W42b Confirm Measurements

All rows compare against n4-c2 in the same batch, with C2 and the explicit Silo loss profile (lower bounds off, 5 percent loss threshold).
Costs show baseline -> arm; CPU is process seconds per decimal GB in the application measurement window.
Server+client CPU is the sum of the reported peer medians, not the median of per-run sums.
W42b runs n=10 per arm with competing CUBIC and household ping in all eight cells; no guard is unmeasured.
The competitor-share guard uses N4 bytes / (N4 bytes + CUBIC bytes); both reported share medians appear below.
Goodput 95% intervals use whole-run independent-arm bootstrap, 10,000 resamples, seed 20261030. Drops below are medians of the per-run drop counts in pacer-guards.json, including configured random loss in the WAN cells.
The typed Go research list owns status and experimental values; this snapshot preserves the source measurements.

## Source Artifacts

- W42b confirm pacer-guards.json; SHA-256 4e49a59e57e3f5b4309836f77fb811d716f9eaa290a90874763fc88e905ec991.
- W42b confirm table.tsv; SHA-256 a6ef80c058335762e134c0a4769cd33f9945a71e583e2e84964a1c1d42607740.

## Per-Cell Results

| Measurement ID | Cell | Arm | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops |
|---|---|---|---|---|---|---|---|---|---|
| W42b/confirm-n10 | fifo-deep-25 | n4-pacer-ack4 | 0.990398 [0.907104, 1.026421] | 339.000 -> 339.000 | 0.575675 -> 0.573552 | 0.424325 -> 0.426448 | 31.582912 -> 27.511707 | 69.127514 -> 58.565609 | 372.0 -> 319.0 |
| W42b/confirm-n10 | fifo-deep-50 | n4-pacer-ack4 | 1.206904 [1.127228, 1.305721] | 338.500 -> 339.000 | 0.470552 -> 0.569898 | 0.529448 -> 0.430102 | 26.249233 -> 19.161096 | 61.974513 -> 47.460461 | 459.5 -> 854.5 |
| W42b/confirm-n10 | fifo-deep-100 | n4-pacer-ack4 | 1.247453 [1.123465, 1.303705] | 337.500 -> 340.000 | 0.491925 -> 0.614166 | 0.508075 -> 0.385834 | 21.902010 -> 14.689101 | 52.910606 -> 39.505462 | 608.0 -> 1119.0 |
| W42b/confirm-n10 | wan-200 | n4-pacer-ack4 | 1.042150 [1.039890, 1.045156] | 70.550 -> 70.150 | 0.985387 -> 0.985610 | 0.014613 -> 0.014390 | 18.099489 -> 13.255145 | 38.227231 -> 30.511240 | 4956.0 -> 5220.5 |
| W42b/confirm-n10 | police-25 | n4-pacer-ack4 | 1.020605 [1.001578, 1.042997] | 40.000 -> 40.000 | 0.886006 -> 0.888313 | 0.113994 -> 0.111687 | 25.276819 -> 23.328736 | 47.909583 -> 42.643490 | 502.5 -> 447.5 |
| W42b/confirm-n10 | police-50 | n4-pacer-ack4 | 1.068936 [1.042979, 1.084949] | 40.000 -> 40.000 | 0.881161 -> 0.887582 | 0.118839 -> 0.112418 | 15.723231 -> 15.418482 | 30.249596 -> 28.868793 | 621.5 -> 591.0 |
| W42b/confirm-n10 | police-100 | n4-pacer-ack4 | 1.105925 [1.068615, 1.123288] | 40.000 -> 40.000 | 0.877400 -> 0.889122 | 0.122600 -> 0.110878 | 10.801199 -> 10.120728 | 20.873124 -> 19.259237 | 868.0 -> 805.5 |
| W42b/confirm-n10 | wan-1000 | n4-pacer-ack4 | 1.146500 [1.117554, 1.161462] | 59.650 -> 65.250 | 0.996471 -> 0.997563 | 0.003529 -> 0.002437 | 13.063060 -> 13.014796 | 25.648232 -> 23.882669 | 27059.5 -> 31698.0 |
