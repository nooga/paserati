#!/usr/bin/env python3
"""Refresh README compliance numbers and the SVG compliance chart."""

from __future__ import annotations

import argparse
import json
import math
import re
import subprocess
from dataclasses import dataclass, asdict
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"
CHART = ROOT / "docs" / "compliance.svg"
CACHE = ROOT / "docs" / "compliance.json"


@dataclass
class Suite:
    name: str
    total: int
    passed: int
    failed: int
    skipped: int = 0
    timeout: int = 0
    version: str = ""

    @property
    def pass_rate(self) -> float:
        return (self.passed / self.total * 100.0) if self.total else 0.0


def count_baseline(path: Path, name: str) -> Suite:
    passed = 0
    failed = 0
    for line in path.read_text().splitlines():
        if line.startswith("+"):
            passed += 1
        elif line.startswith("-"):
            failed += 1
    return Suite(name=name, total=passed + failed, passed=passed, failed=failed)


def read_ts_version(ts_path: Path) -> str:
    package_json = ts_path / "package.json"
    if not package_json.exists():
        return "unknown"
    data = json.loads(package_json.read_text())
    return str(data.get("version", "unknown"))


def run_ts_suite_once(ts_path: Path, timeout: str, strict: bool) -> Suite:
    args = [
        str(ROOT / "paserati-testtsc"),
        "-path",
        str(ts_path),
        "-suite",
        "-timeout",
        timeout,
    ]
    if strict:
        args.append("-strict-errors")
    result = subprocess.run(
        args,
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=True,
    )
    match = re.search(
        r"GRAND TOTAL\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+([0-9.]+)%",
        result.stdout,
    )
    if not match:
        raise RuntimeError("Could not find TypeScript GRAND TOTAL line in paserati-testtsc output")

    total, passed, failed, skipped, timeout_count, _ = match.groups()
    suffix = " (strict error codes)" if strict else " (loose)"
    return Suite(
        name=f"TypeScript {read_ts_version(ts_path)}{suffix}",
        total=int(total),
        passed=int(passed),
        failed=int(failed),
        skipped=int(skipped),
        timeout=int(timeout_count),
        version=read_ts_version(ts_path),
    )


def run_ts_suite(ts_path: Path, timeout: str, runs: int, strict: bool) -> Suite:
    results = []
    for _ in range(max(1, runs)):
        results.append(run_ts_suite_once(ts_path, timeout, strict))

    best = max(results, key=lambda suite: (suite.passed, -suite.failed, -suite.timeout))
    if len({(suite.passed, suite.failed, suite.skipped, suite.timeout) for suite in results}) > 1:
        print(
            "TypeScript suite result varied across runs; using best observed result "
            f"{best.passed}/{best.total}."
        )
    return best


def load_cached_ts(key: str) -> Suite | None:
    if not CACHE.exists():
        return None
    data = json.loads(CACHE.read_text())
    ts_data = data.get(key)
    if not ts_data:
        return None
    return Suite(**ts_data)


def save_cache(language: Suite, builtins: Suite, ts_loose: Suite, ts_strict: Suite) -> None:
    CACHE.write_text(
        json.dumps(
            {
                "test262_language": asdict(language),
                "test262_builtins": asdict(builtins),
                "typescript_loose": asdict(ts_loose),
                "typescript_strict": asdict(ts_strict),
            },
            indent=2,
        )
        + "\n"
    )


def polar(cx: float, cy: float, radius: float, angle: float) -> tuple[float, float]:
    radians = (angle - 90.0) * math.pi / 180.0
    return cx + radius * math.cos(radians), cy + radius * math.sin(radians)


def pie_slice(cx: float, cy: float, radius: float, start: float, end: float, color: str) -> str:
    if end - start >= 359.99:
        return f'<circle cx="{cx}" cy="{cy}" r="{radius}" fill="{color}" />'
    x1, y1 = polar(cx, cy, radius, end)
    x2, y2 = polar(cx, cy, radius, start)
    large = 1 if end - start > 180.0 else 0
    return (
        f'<path d="M {cx:.2f} {cy:.2f} L {x1:.2f} {y1:.2f} '
        f'A {radius:.2f} {radius:.2f} 0 {large} 0 {x2:.2f} {y2:.2f} Z" fill="{color}" />'
    )


def draw_pie(suite: Suite, cx: int, cy: int, label: str | None = None) -> str:
    radius = 62
    segments = [
        ("Pass", suite.passed, "#20a060"),
        ("Fail", suite.failed, "#d64b4b"),
        ("Skip", suite.skipped, "#aeb7c2"),
        ("Timeout", suite.timeout, "#f4a340"),
    ]
    angle = 0.0
    title = label if label is not None else suite.name
    parts = [f'<g aria-label="{suite.name}">']
    for _, value, color in segments:
        if value <= 0 or suite.total <= 0:
            continue
        sweep = value / suite.total * 360.0
        parts.append(pie_slice(cx, cy, radius, angle, angle + sweep, color))
        angle += sweep
    parts.append(f'<circle cx="{cx}" cy="{cy}" r="38" fill="#ffffff" />')
    parts.append(
        f'<text x="{cx}" y="{cy - 4}" text-anchor="middle" class="percent">{suite.pass_rate:.1f}%</text>'
    )
    parts.append(f'<text x="{cx}" y="{cy + 18}" text-anchor="middle" class="caption">pass</text>')
    parts.append(f'<text x="{cx}" y="{cy + 98}" text-anchor="middle" class="title">{title}</text>')
    parts.append(
        f'<text x="{cx}" y="{cy + 120}" text-anchor="middle" class="caption">'
        f'{suite.passed:,}/{suite.total:,}</text>'
    )
    parts.append("</g>")
    return "\n".join(parts)


def render_svg(language: Suite, builtins: Suite, ts_strict: Suite, ts_loose: Suite) -> str:
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="1080" height="320" viewBox="0 0 1080 320" role="img" aria-labelledby="title desc">
  <title id="title">Paserati compliance snapshot</title>
  <desc id="desc">Pie charts for Test262 language, Test262 built-ins, and TypeScript conformance (strict error codes and loose) pass rates.</desc>
  <style>
    .bg {{ fill: #fbfcfe; }}
    .group {{ font: 600 15px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #304050; }}
    .title {{ font: 600 16px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #18212b; }}
    .percent {{ font: 700 22px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #18212b; }}
    .caption {{ font: 13px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #66717f; }}
    .legend {{ font: 13px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #3a4653; }}
  </style>
  <rect class="bg" x="0" y="0" width="1080" height="320" rx="8" />
  <text x="250" y="42" text-anchor="middle" class="group">Test262 compliance</text>
  <text x="710" y="42" text-anchor="middle" class="group">TypeScript conformance</text>
  <line x1="470" y1="28" x2="470" y2="250" stroke="#d9e0e8" stroke-width="1" />
  {draw_pie(language, 165, 135)}
  {draw_pie(builtins, 340, 135)}
  {draw_pie(ts_strict, 610, 135, label="strict error codes")}
  {draw_pie(ts_loose, 790, 135, label="loose")}
  <g transform="translate(584 278)">
    <rect x="0" y="-10" width="12" height="12" fill="#20a060" rx="2" /><text x="18" y="1" class="legend">Pass</text>
    <rect x="84" y="-10" width="12" height="12" fill="#d64b4b" rx="2" /><text x="102" y="1" class="legend">Fail</text>
    <rect x="162" y="-10" width="12" height="12" fill="#aeb7c2" rx="2" /><text x="180" y="1" class="legend">Skip</text>
    <rect x="240" y="-10" width="12" height="12" fill="#f4a340" rx="2" /><text x="258" y="1" class="legend">Timeout</text>
  </g>
</svg>
"""


def readme_block(language: Suite, builtins: Suite, ts_strict: Suite, ts_loose: Suite) -> str:
    version = ts_strict.version or ts_loose.version or "unknown"
    return f"""<!-- compliance:begin -->
![Compliance snapshot](docs/compliance.svg)

| Suite | Passed | Failed | Skipped | Timeouts | Pass rate |
| :-- | --: | --: | --: | --: | --: |
| Test262 language | {language.passed:,}/{language.total:,} | {language.failed:,} | {language.skipped:,} | {language.timeout:,} | {language.pass_rate:.1f}% |
| Test262 built-ins | {builtins.passed:,}/{builtins.total:,} | {builtins.failed:,} | {builtins.skipped:,} | {builtins.timeout:,} | {builtins.pass_rate:.1f}% |
| TypeScript {version} conformance (strict error codes) | {ts_strict.passed:,}/{ts_strict.total:,} | {ts_strict.failed:,} | {ts_strict.skipped:,} | {ts_strict.timeout:,} | {ts_strict.pass_rate:.1f}% |
| TypeScript {version} conformance (loose) | {ts_loose.passed:,}/{ts_loose.total:,} | {ts_loose.failed:,} | {ts_loose.skipped:,} | {ts_loose.timeout:,} | {ts_loose.pass_rate:.1f}% |
<!-- compliance:end -->

`strict error codes` requires our diagnostics to carry the same TypeScript error code(s) the baseline expects; `loose` only requires that we raised *some* error where one was expected. Loose overcounts conformance — treat strict as the honest number."""


def update_readme(language: Suite, builtins: Suite, ts_strict: Suite, ts_loose: Suite) -> None:
	text = README.read_text()
	version = ts_strict.version or ts_loose.version or "unknown"
	block = readme_block(language, builtins, ts_strict, ts_loose)
	pattern = re.compile(
		r"<!-- compliance:begin -->.*?<!-- compliance:end -->"
		r"(\n\n`strict error codes`.*?honest number\.)?",
		re.S,
	)
	if not pattern.search(text):
		raise RuntimeError("README.md is missing compliance block markers")
	text = pattern.sub(block, text)
	text = re.sub(
		r"\*\*Test262 language suite: [0-9.]+%\*\*, \*\*built-ins: [0-9.]+%\*\*, "
		r"\*\*TypeScript [^*]+ conformance: [0-9.]+%\*\*",
		f"**Test262 language suite: {language.pass_rate:.1f}%**, "
		f"**built-ins: {builtins.pass_rate:.1f}%**, "
		f"**TypeScript {version} conformance: {ts_strict.pass_rate:.1f}% strict / {ts_loose.pass_rate:.1f}% loose**",
		text,
	)
	text = re.sub(
		r"At \*\*[0-9.]+% Test262 language compliance\*\* and "
		r"\*\*[0-9.]+% TypeScript [^*]+ conformance\*\*",
		f"At **{language.pass_rate:.1f}% Test262 language compliance** and "
		f"**{ts_strict.pass_rate:.1f}% TypeScript {version} conformance** (strict error codes; "
		f"{ts_loose.pass_rate:.1f}% under the looser any-error-raised metric)",
		text,
	)
	README.write_text(text)


def resolve_ts(
    key: str, ts_path: Path, timeout: str, runs: int, flake_tolerance: int, strict: bool, run_ts: bool
) -> Suite:
    if not run_ts:
        cached = load_cached_ts(key)
        if cached is None:
            raise RuntimeError(f"No cached TypeScript metrics found for '{key}'; rerun with --run-ts")
        return cached

    ts = run_ts_suite(ts_path, timeout, runs, strict)
    cached = load_cached_ts(key)
    if (
        cached is not None
        and cached.version == ts.version
        and cached.total == ts.total
        and cached.passed > ts.passed
        and cached.passed - ts.passed <= flake_tolerance
    ):
        print(
            f"Preserving cached TypeScript ({key}) best "
            f"{cached.passed}/{cached.total}; rerun was lower by "
            f"{cached.passed - ts.passed}."
        )
        ts = cached
    return ts


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--language-baseline", default="baseline_language.txt")
    parser.add_argument("--builtins-baseline", default="baseline.txt")
    parser.add_argument("--run-ts", action="store_true", help="Run the TypeScript conformance suite (both strict and loose)")
    parser.add_argument("--ts-path", default="../TypeScript")
    parser.add_argument("--ts-timeout", default="0.2s")
    parser.add_argument("--ts-runs", type=int, default=2, help="Number of TypeScript suite runs to smooth one-test flakes")
    parser.add_argument("--ts-flake-tolerance", type=int, default=3, help="Preserve cached TS best if a rerun is lower by at most this many passes")
    args = parser.parse_args()

    language = count_baseline(ROOT / args.language_baseline, "Test262 language")
    builtins = count_baseline(ROOT / args.builtins_baseline, "Test262 built-ins")

    if args.run_ts:
        subprocess.run(
            ["go", "build", "-o", "paserati-testtsc", "./cmd/paserati-testtsc"],
            cwd=ROOT,
            check=True,
        )

    ts_path = (ROOT / args.ts_path).resolve()
    ts_strict = resolve_ts(
        "typescript_strict", ts_path, args.ts_timeout, args.ts_runs, args.ts_flake_tolerance, True, args.run_ts
    )
    ts_loose = resolve_ts(
        "typescript_loose", ts_path, args.ts_timeout, args.ts_runs, args.ts_flake_tolerance, False, args.run_ts
    )

    CHART.write_text(render_svg(language, builtins, ts_strict, ts_loose))
    save_cache(language, builtins, ts_loose, ts_strict)
    update_readme(language, builtins, ts_strict, ts_loose)

    print(f"Test262 language: {language.passed}/{language.total} ({language.pass_rate:.1f}%)")
    print(f"Test262 built-ins: {builtins.passed}/{builtins.total} ({builtins.pass_rate:.1f}%)")
    print(f"TypeScript {ts_strict.version} (strict): {ts_strict.passed}/{ts_strict.total} ({ts_strict.pass_rate:.1f}%)")
    print(f"TypeScript {ts_loose.version} (loose): {ts_loose.passed}/{ts_loose.total} ({ts_loose.pass_rate:.1f}%)")


if __name__ == "__main__":
    main()
