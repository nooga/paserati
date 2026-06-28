#!/usr/bin/env bash
# Compare two macro-test262 passes (same-runner A/B) → markdown delta table.
#
# Usage: scripts/macro-test262-compare.sh <base.merged.json> <head.merged.json>
#
# INTERSECTION compare: sums execution time only over tests that passed (and
# didn't time out) on BOTH sides. A test that passes on one side but times out
# or fails on the other is excluded from both sums, so boundary-churn (a slow
# test flipping pass<->timeout between passes) can't move the delta. That churn
# was the dominant noise source when summing each side's passing set independently.
#
# Δ% is on raw summed ns — both halves ran on one runner, so there's nothing to
# normalize. The excluded-count is reported so churn is visible, not silent.
set -euo pipefail

base="${1:?usage: macro-test262-compare.sh <base.merged.json> <head.merged.json>}"
head="${2:?usage: macro-test262-compare.sh <base.merged.json> <head.merged.json>}"

jq -rn --slurpfile b "$base" --slurpfile h "$head" '
  def suite($p): ($p | capture("/test/(?<s>[^/]+)/") | .s) // "unknown";
  def passing($doc): [ $doc[0].results[] | select(.passed and (.timedOut|not)) ];

  (passing($b) | map({(.path): .duration}) | add // {}) as $bmap |
  (passing($h) | map({(.path): .duration}) | add // {}) as $hmap |
  [ $bmap | keys[] | select($hmap[.] != null) ] as $common |

  # Per-suite + total sums over the common (both-passing) set.
  (reduce $common[] as $p ({};
     suite($p) as $s |
     .[$s].base += $bmap[$p] | .[$s].head += $hmap[$p] |
     .["total"].base += $bmap[$p] | .["total"].head += $hmap[$p]
   )) as $sum |

  def ms($v): ($v / 1e6 * 10 | round / 10);
  def pct($a; $b): (if $a > 0 then (($b - $a) / $a * 100 * 100 | round / 100) else 0 end);

  ( ($bmap|length) - ($common|length) ) as $base_only |
  ( ($hmap|length) - ($common|length) ) as $head_only |

  "Intersection: \($common|length) tests passing on both sides " +
    "(excluded — base-only \($base_only), head-only \($head_only)).",
  "",
  "| Series | Base (ms) | Head (ms) | Δ% |",
  "|---|---:|---:|---:|",
  ( ($sum | keys | sort)[] |
    . as $k | $sum[$k] as $e |
    pct($e.base; $e.head) as $d |
    "| `test262.\($k)` | \(ms($e.base)) | \(ms($e.head)) | \(if $d >= 0 then "+\($d)" else "\($d)" end) |"
  )
'
