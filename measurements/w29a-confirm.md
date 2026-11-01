# W29a Confirm Measurements

These are copied report medians at n=10 per arm, relative to n4-c2 with the explicit Silo loss profile (lower bounds off, 5 percent loss threshold).
Goodput ratios use independent-arm whole-run bootstrap 95% intervals: 10,000 resamples, seed 20261029.
CPU is process CPU in the application measurement window, in seconds per decimal GB.
Server+client CPU is the sum of the separately reported server and client medians, not a median of per-run sums.
Competitor share is unmeasured: these confirms run no competing download. Household ping is unmeasured at wan-200.
Costs show baseline -> arm. Bottleneck drops are medians, including configured random loss at wan-200.
The typed research list in internal/congestion/anchor_test.go owns status, anchor and experimental values; this file preserves measurement evidence.

## Source Reports

- W29a/confirm-deep-wan/n4-tune-quiche/quiche-tuning.json; SHA-256 0abe2a676297fd48fdde7594f4f289173476bfe85f27a62c8ec17e95c00afee6.
- W29a/confirm-police/n4-tune-quiche/quiche-tuning.json; SHA-256 23357bd5d80f9e1c4b9c3881ef61e513b72e95ba54fd12c45310be0d6b615f35.
- W29a/confirm-police50/n4-tune-quiche/quiche-tuning.json; SHA-256 02ab67563672304b63b0510b464972830aacebb65ab89f8f1ba9bacaa67da32f.

## Per-Cell Results

| Measurement ID | Cell | Arm | Goodput Ratio [95% CI] | Ping p95 ms | Competitor Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops |
|---|---|---|---|---|---|---|---|---|
| W29a/confirm-deep-wan | fifo-deep-25 | n4-pacer-catchup | 1.023435 [1.022883, 1.026914] | 94.750 -> 93.200 | unmeasured | 23.095347 -> 25.251741 | 58.952741 -> 57.511123 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | fifo-deep-50 | n4-pacer-catchup | 1.035729 [1.032515, 1.041484] | 98.250 -> 88.750 | unmeasured | 22.123442 -> 23.089075 | 53.990989 -> 52.490880 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | fifo-deep-100 | n4-pacer-catchup | 1.041747 [1.037661, 1.045183] | 98.450 -> 87.500 | unmeasured | 18.977052 -> 20.781495 | 46.452318 -> 45.903755 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | wan-200 | n4-pacer-catchup | 1.040649 [1.038319, 1.044593] | unmeasured | unmeasured | 17.521959 -> 19.721513 | 37.112749 -> 39.469894 | 4943.0 -> 5129.0 |
| W29a/confirm-police | police-25 | n4-pacer-catchup | 1.024194 [1.017411, 1.031669] | 40.000 -> 40.000 | unmeasured | 25.438197 -> 24.549573 | 48.163202 -> 45.871201 | 285.0 -> 458.5 |
| W29a/confirm-police | police-100 | n4-pacer-catchup | 1.125387 [1.110590, 1.139633] | 40.000 -> 40.000 | unmeasured | 10.184706 -> 10.443545 | 19.491200 -> 19.805654 | 661.0 -> 585.5 |
| W29a/confirm-police50 | police-50 | n4-pacer-catchup | 1.069945 [1.063795, 1.081274] | 40.000 -> 40.000 | unmeasured | 15.154407 -> 15.337595 | 29.117327 -> 28.899171 | 369.0 -> 438.0 |
| W29a/confirm-deep-wan | fifo-deep-25 | n4-probebw-exits | 1.026745 [1.026646, 1.029087] | 94.750 -> 94.000 | unmeasured | 23.095347 -> 23.609290 | 58.952741 -> 55.973380 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | fifo-deep-50 | n4-probebw-exits | 1.037358 [1.033706, 1.042076] | 98.250 -> 120.500 | unmeasured | 22.123442 -> 21.256780 | 53.990989 -> 52.593586 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | fifo-deep-100 | n4-probebw-exits | 1.031059 [1.029104, 1.033862] | 98.450 -> 119.000 | unmeasured | 18.977052 -> 18.655319 | 46.452318 -> 45.815044 | 0.0 -> 0.0 |
| W29a/confirm-deep-wan | wan-200 | n4-probebw-exits | 1.016036 [1.013288, 1.017714] | unmeasured | unmeasured | 17.521959 -> 17.427825 | 37.112749 -> 36.940734 | 4943.0 -> 5046.5 |
| W29a/confirm-police | police-25 | n4-probeup-anchor | 1.013829 [1.003003, 1.023060] | 40.000 -> 40.000 | unmeasured | 25.438197 -> 22.744889 | 48.163202 -> 43.508086 | 285.0 -> 1029.0 |
| W29a/confirm-police | police-100 | n4-probeup-anchor | 1.008645 [0.991490, 1.029429] | 40.000 -> 40.000 | unmeasured | 10.184706 -> 10.693970 | 19.491200 -> 21.017458 | 661.0 -> 1636.0 |
