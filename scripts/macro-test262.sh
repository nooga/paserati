#!/usr/bin/env bash
# Run the Test262 macro-benchmark for the current checkout and emit the raw
# per-series execution-time records plus the correctness stats.
#
# Usage: scripts/macro-test262.sh <records-out.jsonl> <stats-out.json>
#
# Writes:
#   <records-out>.jsonl   StreamRecords: total + per top-level suite, ns_per_op =
#                         summed execution time over passing, non-timed-out tests
#   <stats-out>.json      Test262 pass/fail/timeout counts (the correctness signal)
#
# This is one same-runner A/B half: the caller runs it on the merge-base and the
# head, then diffs the raw sums. No anchor normalization — both halves run on one
# machine, so there's nothing to cancel (the anchor is for a cross-runner timeline).
#
# SHARDED: each second-level directory runs as its own process, so VM memory
# (shapes, IC caches, interned strings — which otherwise grow unbounded across the
# corpus and OOM a 7GB runner) is released between shards. Per-test durations are
# process-independent, so sharding doesn't change the metric.
#
# Env:
#   PER_TEST_TIMEOUT  per-test safety bound (default 0.2s). Timed-out tests are
#                     excluded from the timing sum and counted separately, so a
#                     tight bound just identifies them faster — it doesn't change
#                     the speed metric, only the wasted wall-time on hangs.
#   TEST262_CHAPTERS  space-separated chapters to shard under (default
#                     "built-ins language" — the JS-execution core; intl402,
#                     staging and annexB are skipped). Accepts deeper paths too,
#                     e.g. "built-ins/Math" for a quick local run.
set -euo pipefail

records_out="${1:?usage: macro-test262.sh <records.jsonl> <stats.json>}"
stats_out="${2:?usage: macro-test262.sh <records.jsonl> <stats.json>}"
merged="${records_out%.jsonl}.merged.json"
shard_dir="${records_out%.jsonl}.shards"

timeout="${PER_TEST_TIMEOUT:-0.2s}"
chapters="${TEST262_CHAPTERS:-built-ins language}"

# bench-test262 -out appends; clear stale output so reruns don't accumulate.
rm -f "$records_out" "$merged"
rm -rf "$shard_dir"
mkdir -p "$shard_dir"

go build -o ./paserati-test262 ./cmd/paserati-test262
go build -o ./bench-test262 ./cmd/bench-test262

# Enumerate shards: each second-level directory under each chapter.
shards=()
for ch in $chapters; do
  while IFS= read -r d; do
    shards+=("${ch}/$(basename "$d")")
  done < <(find "test262/test/${ch}" -mindepth 1 -maxdepth 1 -type d | sort)
done
echo "macro-test262: ${#shards[@]} shards across: ${chapters}"

# Sharding by second-level dir misses any .js directly under a chapter root.
# built-ins/ and language/ have none, but surface it rather than drop silently.
for ch in $chapters; do
  direct="$(find "test262/test/${ch}" -maxdepth 1 -type f -name '*.js' | wc -l | tr -d ' ')"
  [ "$direct" -gt 0 ] && echo "macro-test262: WARNING — ${direct} root-level .js under ${ch}/ are NOT sharded" >&2
done

# Run each shard in its own process. Discard stderr: stack-overflow tests dump
# the whole VM stack there, which unbounded floods the CI log enough to kill the
# job. The runner exits non-zero on any failure (expected), so ignore exit codes.
i=0
for sub in "${shards[@]}"; do
  raw="${shard_dir}/$(printf '%04d' "$i").json"
  ./paserati-test262 -path ./test262 -timeout "$timeout" -subpath "${sub}/**" -json > "$raw" 2>/dev/null || true
  i=$((i + 1))
done

# Merge shard outputs: concatenate per-test results, sum the stats counters.
jq -s '{
  stats: {
    Total:    (map(.stats.Total)    | add // 0),
    Passed:   (map(.stats.Passed)   | add // 0),
    Failed:   (map(.stats.Failed)   | add // 0),
    Timeouts: (map(.stats.Timeouts) | add // 0),
    Skipped:  (map(.stats.Skipped)  | add // 0),
    Duration: (map(.stats.Duration) | add // 0)
  },
  results: (map(.results) | add // [])
}' "$shard_dir"/*.json > "$merged"

./bench-test262 -in "$merged" -out "$records_out"
jq '.stats' "$merged" > "$stats_out"

echo "macro-test262: wrote $records_out and $stats_out"
