# Perf

`cmd/bench-ratchet` runs the Go benchmarks and normalizes each against a frozen
calibration anchor (`BenchmarkRatchetAnchor`, a pure-CPU FMA loop in `pkg/vm`),
so a benchmark reads as a multiple of the anchor's ns/op rather than an absolute
time.

## PR check (`perf-pr.yml`)

GitHub's shared runners vary by CPU tier, which makes comparing against a stored
baseline unreliable (an ALU anchor and a pointer-chasing lookup don't scale the
same across CPUs). So the PR check is a **same-runner A/B**: it benches the PR's
merge-base and its head on one runner, back to back, and posts the delta to the
run summary (and a sticky PR comment where the token allows).

It's **informational — it never fails the PR**, and it's opt-in via the `perf`
label, since a two-pass bench costs runner minutes. Add the label to run it;
remove it to stop.

## Local use

```bash
go run ./cmd/bench-ratchet show     # capture + print, no write
go run ./cmd/bench-ratchet check    # compare current vs a -baseline JSON
go run ./cmd/bench-ratchet update   # ratchet a baseline toward current
```

`update` is a ratchet — each metric only moves toward faster / fewer allocs; a
removed benchmark keeps its bar unless `-force`. `docs/perf/.runs/` (raw `.jsonl`
captures) is gitignored.
