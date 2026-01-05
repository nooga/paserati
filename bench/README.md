## Benchmarks

This directory contains a tiny, deterministic micro-benchmark meant to be run by both:

- `./paserati` (native TS frontend + bytecode VM)
- `./gojac` (Goja-based runner binary in this repo)

### Run with hyperfine

From the repo root:

```bash
./bench/hyperfine.sh           # runs all benchmarks
./bench/hyperfine.sh bench     # arithmetic/array microbench
./bench/hyperfine.sh objects   # object-heavy microbench
```

Outputs go to:

- `bench/out/*.md`
- `bench/out/*.json`


