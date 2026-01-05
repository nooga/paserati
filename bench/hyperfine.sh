#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

PASERATI="${ROOT_DIR}/paserati"
GOJAC="${ROOT_DIR}/gojac"

BENCH_NAME="${1:-all}"

if [[ ! -x "${PASERATI}" ]]; then
  echo "error: missing executable: ${PASERATI}"
  echo "hint: build it (however you normally do) so ./paserati exists"
  exit 1
fi

if [[ ! -x "${GOJAC}" ]]; then
  echo "error: missing executable: ${GOJAC}"
  echo "hint: build it (however you normally do) so ./gojac exists"
  exit 1
fi

if ! command -v hyperfine >/dev/null 2>&1; then
  echo "error: hyperfine not found on PATH"
  echo "hint (macOS): brew install hyperfine"
  echo "hint (cargo): cargo install hyperfine"
  exit 1
fi

mkdir -p "${ROOT_DIR}/bench/out"

run_one () {
  local name="$1"
  local script="$2"
  local out_md="${ROOT_DIR}/bench/out/${name}.md"
  local out_json="${ROOT_DIR}/bench/out/${name}.json"

  if [[ ! -f "${script}" ]]; then
    echo "error: missing benchmark script: ${script}"
    exit 1
  fi

  echo "Benchmark: ${name}"
  echo "  - ${PASERATI} ${script}"
  echo "  - ${GOJAC} ${script}"
  echo

  hyperfine \
    --warmup 3 \
    --min-runs 20 \
    --style full \
    --export-markdown "${out_md}" \
    --export-json "${out_json}" \
    --command-name "paserati" "${PASERATI} --no-typecheck ${script} > /dev/null 2>&1" \
    --command-name "gojac" "${GOJAC} ${script} > /dev/null 2>&1"

  echo
  echo "Wrote:"
  echo "  - ${out_md}"
  echo "  - ${out_json}"
  echo
}

case "${BENCH_NAME}" in
  all)
    run_one "bench" "${ROOT_DIR}/bench/bench.js"
    run_one "objects" "${ROOT_DIR}/bench/objects.js"
    ;;
  bench|bench.js)
    run_one "bench" "${ROOT_DIR}/bench/bench.js"
    ;;
  objects|objects.js)
    run_one "objects" "${ROOT_DIR}/bench/objects.js"
    ;;
  *)
    echo "usage: $0 [all|bench|objects]"
    exit 64
    ;;
esac


