# W42a Pacer Guard Measurements

These n=10 measurements compare n4-pacer-catchup against n4-c2 with a competing CUBIC download and household ping in all seven cells.
W29a solo confirms remain separate measurements; each W42a value uses its own batch baseline.
Costs show baseline -> arm. CPU is process seconds per decimal GB in the application measurement window.
Server+client CPU is the sum of the reported peer medians, not the median of per-run sums.
The competitor-share guard uses N4 bytes / (N4 bytes + CUBIC bytes); both reported share medians appear below.
Goodput 95% intervals use whole-run independent-arm bootstrap, 10,000 resamples, seed 20261030.
Bottleneck drops are medians of the per-run counts in pacer-guards.json, including configured random loss at wan-200.
The typed Go research list owns status and experimental values; this snapshot preserves the source measurements.

## Source Artifacts

- W42a guards pacer-guards.json; SHA-256 b759c0a9610e3131025d12920a3ab8c72a4dc3e363d39b924f269821f016617d.
- W42a guards table.tsv; SHA-256 a3de13b7da1c2107d4cdd87947b87a392c3982bc7d37b5c046bd328a9d0bf998.

## Per-Cell Results

| Measurement ID | Cell | Arm | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops |
|---|---|---|---|---|---|---|---|---|---|
| W42a/guards-n10 | fifo-deep-25 | n4-pacer-catchup | 0.955057 [0.897065, 0.994486] | 339.000 -> 339.000 | 0.579734 -> 0.555846 | 0.420266 -> 0.444154 | 30.182815 -> 31.391419 | 66.511510 -> 66.501995 | 428.5 -> 386.5 |
| W42a/guards-n10 | fifo-deep-50 | n4-pacer-catchup | 1.298443 [1.210310, 1.342627] | 339.000 -> 340.000 | 0.471315 -> 0.612535 | 0.528685 -> 0.387465 | 25.418496 -> 25.640900 | 60.277774 -> 56.992322 | 501.0 -> 823.0 |
| W42a/guards-n10 | fifo-deep-100 | n4-pacer-catchup | 1.250345 [1.170691, 1.425831] | 336.500 -> 339.000 | 0.473149 -> 0.593252 | 0.526851 -> 0.406748 | 21.569459 -> 22.103210 | 52.269318 -> 49.961223 | 514.5 -> 921.5 |
| W42a/guards-n10 | wan-200 | n4-pacer-catchup | 1.045751 [1.042473, 1.048549] | 71.500 -> 71.150 | 0.983912 -> 0.985264 | 0.016088 -> 0.014736 | 17.719177 -> 20.078733 | 37.522565 -> 40.234854 | 4943.5 -> 5211.0 |
| W42a/guards-n10 | police-25 | n4-pacer-catchup | 1.016009 [0.995494, 1.023520] | 40.000 -> 40.000 | 0.902290 -> 0.891895 | 0.097710 -> 0.108105 | 24.748968 -> 25.129568 | 46.681869 -> 46.765930 | 558.5 -> 456.5 |
| W42a/guards-n10 | police-50 | n4-pacer-catchup | 1.058015 [1.039034, 1.087516] | 40.000 -> 40.000 | 0.873623 -> 0.873577 | 0.126377 -> 0.126423 | 15.631970 -> 15.921877 | 30.022077 -> 29.938624 | 619.0 -> 558.5 |
| W42a/guards-n10 | police-100 | n4-pacer-catchup | 1.100264 [1.080575, 1.121217] | 40.000 -> 40.000 | 0.880408 -> 0.893180 | 0.119592 -> 0.106820 | 10.566045 -> 10.738549 | 20.396907 -> 20.454566 | 950.5 -> 792.5 |
