# W43a Loop Round 4 Screen Measurements

These n=5 screens compare each arm against n4-c2 with competing CUBIC and household ping in all eight cells.
Pacer + ACK4 is a reference arm, not the denominator of the reported goodput ratios.
Costs show baseline -> arm; CPU is process seconds per decimal GB in the application measurement window.
Server+client CPU is the reported sum of peer medians, not a median of per-run sums.
The competitor-share guard uses N4 bytes / (N4 bytes + CUBIC bytes); both reported share medians appear below.
Goodput 95% intervals use whole-run independent-arm bootstrap, 10,000 resamples, seed 20261030.
Bottleneck drops are medians of the per-run counts in n4-loop-r4.json, including configured random loss in the WAN cells.
The typed Go research list owns status and experimental values; this snapshot preserves source measurements and failed guards.

## Source Artifacts

- W43a screen n4-loop-r4.json; SHA-256 c9b619fc6b527890fcb7b3b1d96b34d50119323243ac7622454714034c8cfe49.
- W43a screen table.tsv; SHA-256 d52e9dade2f4ba29cbfda7fc6033cdb0260a6c79829ee96746f9df77e0764203.

## Per-Cell Results

| Measurement ID | Cell | Arm | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops | Failed Guards |
|---|---|---|---|---|---|---|---|---|---|---|
| W43a/screen-n5 | fifo-deep-25 | n4-pacer-ack4 | 0.937126 [0.718495, 1.111353] | 339.000 -> 339.000 | 0.598890 -> 0.562224 | 0.401110 -> 0.437776 | 29.266039 -> 27.425970 | 65.413391 -> 57.613559 | 397.0 -> 260.0 | goodput, competitor_share |
| W43a/screen-n5 | fifo-deep-50 | n4-pacer-ack4 | 1.243507 [1.077960, 1.308198] | 339.000 -> 339.000 | 0.466602 -> 0.580636 | 0.533398 -> 0.419364 | 25.378138 -> 18.996637 | 60.135386 -> 46.283166 | 510.0 -> 755.0 | competitor_share |
| W43a/screen-n5 | fifo-deep-100 | n4-pacer-ack4 | 1.415492 [1.116215, 2.773127] | 335.000 -> 339.000 | 0.484823 -> 0.694424 | 0.515177 -> 0.305576 | 21.408615 -> 14.243198 | 51.777205 -> 38.421292 | 352.0 -> 1444.0 | competitor_share |
| W43a/screen-n5 | wan-200 | n4-pacer-ack4 | 1.045057 [1.038992, 1.049289] | 70.500 -> 72.200 | 0.985755 -> 0.986146 | 0.014245 -> 0.013854 | 17.606127 -> 12.883846 | 37.233723 -> 29.752854 | 4991.0 -> 5241.0 | none |
| W43a/screen-n5 | police-25 | n4-pacer-ack4 | 0.994744 [0.964853, 1.028487] | 40.000 -> 40.000 | 0.900413 -> 0.876384 | 0.099587 -> 0.123616 | 25.370763 -> 23.132160 | 47.728619 -> 42.062844 | 562.0 -> 440.0 | none |
| W43a/screen-n5 | police-50 | n4-pacer-ack4 | 1.068216 [1.041370, 1.099487] | 40.000 -> 40.000 | 0.877078 -> 0.886249 | 0.122922 -> 0.113751 | 15.746558 -> 15.255887 | 30.036705 -> 28.408590 | 608.0 -> 598.0 | none |
| W43a/screen-n5 | police-100 | n4-pacer-ack4 | 1.104121 [1.058950, 1.158703] | 40.000 -> 40.000 | 0.871711 -> 0.884618 | 0.128289 -> 0.115382 | 10.411251 -> 9.740764 | 20.263628 -> 18.679836 | 856.0 -> 805.0 | none |
| W43a/screen-n5 | wan-1000 | n4-pacer-ack4 | 1.124292 [1.116135, 1.147177] | 61.300 -> 65.400 | 0.996527 -> 0.997878 | 0.003473 -> 0.002122 | 12.877602 -> 12.770190 | 25.269641 -> 23.504350 | 27028.0 -> 32314.0 | household_ping |
| W43a/screen-n5 | fifo-deep-25 | n4-pacer-ack4-1ms | 0.946716 [0.816483, 1.122725] | 339.000 -> 339.000 | 0.598890 -> 0.566913 | 0.401110 -> 0.433087 | 29.266039 -> 26.960931 | 65.413391 -> 57.418829 | 397.0 -> 419.0 | goodput, competitor_share |
| W43a/screen-n5 | fifo-deep-50 | n4-pacer-ack4-1ms | 1.169637 [1.013924, 1.302959] | 339.000 -> 339.000 | 0.466602 -> 0.546008 | 0.533398 -> 0.453992 | 25.378138 -> 19.344821 | 60.135386 -> 46.731315 | 510.0 -> 656.0 | competitor_share |
| W43a/screen-n5 | fifo-deep-100 | n4-pacer-ack4-1ms | 1.310977 [1.100796, 2.568369] | 335.000 -> 339.000 | 0.484823 -> 0.636245 | 0.515177 -> 0.363755 | 21.408615 -> 14.144870 | 51.777205 -> 38.343967 | 352.0 -> 1139.0 | competitor_share |
| W43a/screen-n5 | wan-200 | n4-pacer-ack4-1ms | 1.043094 [1.037040, 1.048690] | 70.500 -> 70.100 | 0.985755 -> 0.985250 | 0.014245 -> 0.014750 | 17.606127 -> 12.757818 | 37.233723 -> 29.702861 | 4991.0 -> 5219.0 | none |
| W43a/screen-n5 | police-25 | n4-pacer-ack4-1ms | 1.001112 [0.987054, 1.035070] | 40.000 -> 40.000 | 0.900413 -> 0.881543 | 0.099587 -> 0.118457 | 25.370763 -> 22.949545 | 47.728619 -> 41.829368 | 562.0 -> 453.0 | none |
| W43a/screen-n5 | police-50 | n4-pacer-ack4-1ms | 1.062830 [1.042869, 1.093943] | 40.000 -> 40.000 | 0.877078 -> 0.882174 | 0.122922 -> 0.117826 | 15.746558 -> 15.294124 | 30.036705 -> 28.412174 | 608.0 -> 576.0 | none |
| W43a/screen-n5 | police-100 | n4-pacer-ack4-1ms | 1.092273 [1.050162, 1.136564] | 40.000 -> 40.000 | 0.871711 -> 0.880386 | 0.128289 -> 0.119614 | 10.411251 -> 9.857504 | 20.263628 -> 18.795922 | 856.0 -> 777.0 | none |
| W43a/screen-n5 | wan-1000 | n4-pacer-ack4-1ms | 1.121954 [1.114852, 1.144792] | 61.300 -> 67.100 | 0.996527 -> 0.997513 | 0.003473 -> 0.002487 | 12.877602 -> 12.878146 | 25.269641 -> 23.612652 | 27028.0 -> 31907.0 | household_ping |
| W43a/screen-n5 | fifo-deep-25 | n4-pacer-ack4-nodebt | 0.932289 [0.739928, 1.105616] | 339.000 -> 339.000 | 0.598890 -> 0.558374 | 0.401110 -> 0.441626 | 29.266039 -> 26.882148 | 65.413391 -> 57.261382 | 397.0 -> 393.0 | goodput, competitor_share |
| W43a/screen-n5 | fifo-deep-50 | n4-pacer-ack4-nodebt | 1.301169 [1.127946, 1.385429] | 339.000 -> 339.000 | 0.466602 -> 0.607618 | 0.533398 -> 0.392382 | 25.378138 -> 18.373205 | 60.135386 -> 45.628884 | 510.0 -> 975.0 | competitor_share |
| W43a/screen-n5 | fifo-deep-100 | n4-pacer-ack4-nodebt | 1.334382 [1.248103, 2.614223] | 335.000 -> 339.000 | 0.484823 -> 0.654445 | 0.515177 -> 0.345555 | 21.408615 -> 14.010130 | 51.777205 -> 38.022232 | 352.0 -> 1144.0 | competitor_share |
| W43a/screen-n5 | wan-200 | n4-pacer-ack4-nodebt | 1.044034 [1.037974, 1.048754] | 70.500 -> 71.700 | 0.985755 -> 0.985609 | 0.014245 -> 0.014391 | 17.606127 -> 12.728400 | 37.233723 -> 29.735430 | 4991.0 -> 5237.0 | none |
| W43a/screen-n5 | police-25 | n4-pacer-ack4-nodebt | 0.996116 [0.933622, 1.029905] | 40.000 -> 40.000 | 0.900413 -> 0.878738 | 0.099587 -> 0.121262 | 25.370763 -> 22.899806 | 47.728619 -> 41.649489 | 562.0 -> 477.0 | goodput |
| W43a/screen-n5 | police-50 | n4-pacer-ack4-nodebt | 1.053136 [1.033365, 1.083965] | 40.000 -> 40.000 | 0.877078 -> 0.871767 | 0.122922 -> 0.128233 | 15.746558 -> 15.148350 | 30.036705 -> 28.322629 | 608.0 -> 573.0 | none |
| W43a/screen-n5 | police-100 | n4-pacer-ack4-nodebt | 1.123063 [1.063637, 1.163394] | 40.000 -> 40.000 | 0.871711 -> 0.900381 | 0.128289 -> 0.099619 | 10.411251 -> 9.820783 | 20.263628 -> 18.675227 | 856.0 -> 867.0 | none |
| W43a/screen-n5 | wan-1000 | n4-pacer-ack4-nodebt | 1.123688 [1.117187, 1.146561] | 61.300 -> 64.400 | 0.996527 -> 0.997689 | 0.003473 -> 0.002311 | 12.877602 -> 12.867617 | 25.269641 -> 23.613026 | 27028.0 -> 31471.0 | household_ping |
