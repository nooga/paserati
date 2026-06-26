# Perf ratchet

`cmd/bench-ratchet` runs the Go benchmarks, normalizes each against a frozen
calibration anchor (`BenchmarkRatchetAnchor`, a pure-CPU FMA loop in `pkg/vm`),
and compares to `baseline.json`. Normalizing against the anchor divides out the
host's raw speed, so a regression flags only when *normalized* work grows — which
is what makes the gate usable on noisy, varying CI runners.

## Local use

```bash
go run ./cmd/bench-ratchet show          # capture + print, no write
go run ./cmd/bench-ratchet check         # compare to baseline, exit 1 on regression
go run ./cmd/bench-ratchet update        # ratchet the baseline toward current (only tightens)
go run ./cmd/bench-ratchet update -force # overwrite the baseline as-is (accept a regression)
```

`update` is a ratchet: each metric only moves toward faster / fewer allocs. A
removed benchmark keeps its bar (rename guard) unless you pass `-force`.

## CI

`.github/workflows/bench.yml` runs `check` on pull requests (anchor-normalized,
`-budget 0.15`) and fails on a regression past the budget.

To move the bar, dispatch the workflow in `update` mode — it re-measures on the
runner and uploads `baseline.json` as an artifact. Download it and commit through
a normal PR, so the baseline stays reviewed source.

The shipped `baseline.json` is anchor-normalized, so its ratios transfer across
machines; `check` notes a machine-fingerprint mismatch until you re-seed on this
repo's own runners (one `update` dispatch + a PR) for the tightest bar.

`docs/perf/.runs/` (raw `.jsonl` captures) is gitignored; only `baseline.json` is
tracked.
