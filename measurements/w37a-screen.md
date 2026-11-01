# W37a Screen Measurements

All rows compare against n4-c2 in the same batch, with C2 and the explicit Silo loss profile (lower bounds off, 5 percent loss threshold).
Costs show baseline -> arm; CPU is process seconds per decimal GB in the application measurement window.
Server+client CPU is the sum of the reported peer medians, not the median of per-run sums.
W37a runs n=5 per arm without a competing download; shares are unmeasured, and ping is unmeasured in the two WAN cells.
The n=5 screen results are separate evidence from the later W42b n=10 confirms.
Goodput 95% intervals and drop medians are copied from table.tsv; drops include configured random loss in the WAN cells.
The typed Go research list owns status and experimental values; this snapshot preserves the source measurements.

## Source Artifacts

- W37a screen table.tsv; SHA-256 59ab9a94ad7ccbe60a1bd9a64303a16ad7066b47004c1a010eb9c7f2a26062df.

## Per-Cell Results

| Measurement ID | Cell | Arm | Goodput Ratio [95% CI] | Ping p95 ms | N4 Share | CUBIC Share | Server CPU s/GB | Server+Client CPU s/GB | Bottleneck Drops |
|---|---|---|---|---|---|---|---|---|---|
| W37a/screen-n5 | fifo-deep-25 | n4-pacer-ack4 | 1.026755 [1.024970, 1.028601] | 95.800 -> 94.800 | unmeasured | unmeasured | 22.262715 -> 20.204617 | 56.776789 -> 47.493014 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-25 | n4-pacer-ack4 | 1.028656 [1.023318, 1.035284] | 40.000 -> 40.000 | unmeasured | unmeasured | 24.416950 -> 21.434651 | 46.113376 -> 39.316611 | 350.0 -> 423.0 |
| W37a/screen-n5 | fifo-deep-50 | n4-pacer-ack4 | 1.041236 [1.032997, 1.044852] | 99.100 -> 91.000 | unmeasured | unmeasured | 21.184513 -> 13.858407 | 52.082059 -> 38.307889 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-50 | n4-pacer-ack4 | 1.058261 [1.054998, 1.075041] | 40.000 -> 40.000 | unmeasured | unmeasured | 14.704604 -> 13.934854 | 28.160766 -> 26.298258 | 378.0 -> 405.0 |
| W37a/screen-n5 | fifo-deep-100 | n4-pacer-ack4 | 1.042594 [1.037195, 1.045236] | 98.500 -> 86.800 | unmeasured | unmeasured | 18.459705 -> 12.752245 | 45.140035 -> 34.022187 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-100 | n4-pacer-ack4 | 1.135807 [1.113026, 1.147332] | 40.000 -> 40.000 | unmeasured | unmeasured | 9.805493 -> 9.178903 | 19.015446 -> 17.467665 | 602.0 -> 576.0 |
| W37a/screen-n5 | wan-200 | n4-pacer-ack4 | 1.040107 [1.038474, 1.045036] | unmeasured | unmeasured | unmeasured | 16.916285 -> 12.092624 | 35.871049 -> 28.283490 | 5042.0 -> 5127.0 |
| W37a/screen-n5 | wan-1000 | n4-pacer-ack4 | 1.119103 [1.116824, 1.124973] | unmeasured | unmeasured | unmeasured | 12.236751 -> 11.841269 | 24.094476 -> 22.101433 | 26996.0 -> 31572.0 |
| W37a/screen-n5 | fifo-deep-25 | n4-client-ack4 | 0.976643 [0.975452, 0.987104] | 95.800 -> 94.400 | unmeasured | unmeasured | 22.262715 -> 21.813468 | 56.776789 -> 52.680431 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-25 | n4-client-ack4 | 1.029372 [1.023394, 1.035170] | 40.000 -> 40.000 | unmeasured | unmeasured | 24.416950 -> 21.103266 | 46.113376 -> 38.540072 | 350.0 -> 384.0 |
| W37a/screen-n5 | fifo-deep-50 | n4-client-ack4 | 0.972198 [0.964505, 0.979905] | 99.100 -> 97.700 | unmeasured | unmeasured | 21.184513 -> 12.801937 | 52.082059 -> 40.120787 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-50 | n4-client-ack4 | 0.976837 [0.964015, 1.000765] | 40.000 -> 40.000 | unmeasured | unmeasured | 14.704604 -> 14.200725 | 28.160766 -> 27.318418 | 378.0 -> 361.0 |
| W37a/screen-n5 | fifo-deep-100 | n4-client-ack4 | 0.964726 [0.960625, 0.972009] | 98.500 -> 98.700 | unmeasured | unmeasured | 18.459705 -> 12.104328 | 45.140035 -> 36.009877 | 0.0 -> 0.0 |
| W37a/screen-n5 | police-100 | n4-client-ack4 | 1.004591 [0.979370, 1.014070] | 40.000 -> 40.000 | unmeasured | unmeasured | 9.805493 -> 9.111460 | 19.015446 -> 17.683228 | 602.0 -> 604.0 |
| W37a/screen-n5 | wan-200 | n4-client-ack4 | 0.976616 [0.975007, 0.982565] | unmeasured | unmeasured | unmeasured | 16.916285 -> 11.058658 | 35.871049 -> 27.538582 | 5042.0 -> 4833.0 |
| W37a/screen-n5 | wan-1000 | n4-client-ack4 | 0.976989 [0.918043, 0.999142] | unmeasured | unmeasured | unmeasured | 12.236751 -> 9.519254 | 24.094476 -> 19.774047 | 26996.0 -> 25725.0 |
