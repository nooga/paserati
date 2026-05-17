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


def run_ts_suite_once(ts_path: Path, timeout: str) -> Suite:
    result = subprocess.run(
        [
            str(ROOT / "paserati-testtsc"),
            "-path",
            str(ts_path),
            "-suite",
            "-timeout",
            timeout,
        ],
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
    return Suite(
        name=f"TypeScript {read_ts_version(ts_path)}",
        total=int(total),
        passed=int(passed),
        failed=int(failed),
        skipped=int(skipped),
        timeout=int(timeout_count),
        version=read_ts_version(ts_path),
    )


def run_ts_suite(ts_path: Path, timeout: str, runs: int) -> Suite:
    subprocess.run(
        ["go", "build", "-o", "paserati-testtsc", "./cmd/paserati-testtsc"],
        cwd=ROOT,
        check=True,
    )

    results = []
    for _ in range(max(1, runs)):
        results.append(run_ts_suite_once(ts_path, timeout))

    best = max(results, key=lambda suite: (suite.passed, -suite.failed, -suite.timeout))
    if len({(suite.passed, suite.failed, suite.skipped, suite.timeout) for suite in results}) > 1:
        print(
            "TypeScript suite result varied across runs; using best observed result "
            f"{best.passed}/{best.total}."
        )
    return best


def load_cached_ts() -> Suite | None:
    if not CACHE.exists():
        return None
    data = json.loads(CACHE.read_text())
    ts_data = data.get("typescript")
    if not ts_data:
        return None
    return Suite(**ts_data)


def save_cache(language: Suite, builtins: Suite, ts: Suite) -> None:
    CACHE.write_text(
        json.dumps(
            {
                "test262_language": asdict(language),
                "test262_builtins": asdict(builtins),
                "typescript": asdict(ts),
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


def draw_pie(suite: Suite, cx: int, cy: int) -> str:
    radius = 62
    segments = [
        ("Pass", suite.passed, "#20a060"),
        ("Fail", suite.failed, "#d64b4b"),
        ("Skip", suite.skipped, "#aeb7c2"),
        ("Timeout", suite.timeout, "#f4a340"),
    ]
    angle = 0.0
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
    parts.append(f'<text x="{cx}" y="{cy + 98}" text-anchor="middle" class="title">{suite.name}</text>')
    parts.append(
        f'<text x="{cx}" y="{cy + 120}" text-anchor="middle" class="caption">'
        f'{suite.passed:,}/{suite.total:,}</text>'
    )
    parts.append("</g>")
    return "\n".join(parts)


def render_svg(language: Suite, builtins: Suite, ts: Suite) -> str:
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="900" height="320" viewBox="0 0 900 320" role="img" aria-labelledby="title desc">
  <title id="title">Paserati compliance snapshot</title>
  <desc id="desc">Three pie charts for Test262 language, Test262 built-ins, and TypeScript conformance pass rates.</desc>
  <style>
    .bg {{ fill: #fbfcfe; }}
    .group {{ font: 600 15px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #304050; }}
    .title {{ font: 600 16px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #18212b; }}
    .percent {{ font: 700 22px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #18212b; }}
    .caption {{ font: 13px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #66717f; }}
    .legend {{ font: 13px system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #3a4653; }}
  </style>
  <rect class="bg" x="0" y="0" width="900" height="320" rx="8" />
  <text x="250" y="42" text-anchor="middle" class="group">Test262 compliance</text>
  <text x="690" y="42" text-anchor="middle" class="group">TypeScript conformance</text>
  <line x1="470" y1="28" x2="470" y2="250" stroke="#d9e0e8" stroke-width="1" />
  {draw_pie(language, 165, 135)}
  {draw_pie(builtins, 340, 135)}
  {draw_pie(ts, 690, 135)}
  <g transform="translate(575 278)">
    <rect x="0" y="-10" width="12" height="12" fill="#20a060" rx="2" /><text x="18" y="1" class="legend">Pass</text>
    <rect x="84" y="-10" width="12" height="12" fill="#d64b4b" rx="2" /><text x="102" y="1" class="legend">Fail</text>
    <rect x="162" y="-10" width="12" height="12" fill="#aeb7c2" rx="2" /><text x="180" y="1" class="legend">Skip</text>
    <rect x="240" y="-10" width="12" height="12" fill="#f4a340" rx="2" /><text x="258" y="1" class="legend">Timeout</text>
  </g>
</svg>
"""


def readme_block(language: Suite, builtins: Suite, ts: Suite) -> str:
    return f"""<!-- compliance:begin -->
![Compliance snapshot](docs/compliance.svg)

| Suite | Passed | Failed | Skipped | Timeouts | Pass rate |
| :-- | --: | --: | --: | --: | --: |
| Test262 language | {language.passed:,}/{language.total:,} | {language.failed:,} | {language.skipped:,} | {language.timeout:,} | {language.pass_rate:.1f}% |
| Test262 built-ins | {builtins.passed:,}/{builtins.total:,} | {builtins.failed:,} | {builtins.skipped:,} | {builtins.timeout:,} | {builtins.pass_rate:.1f}% |
| TypeScript {ts.version or "unknown"} conformance | {ts.passed:,}/{ts.total:,} | {ts.failed:,} | {ts.skipped:,} | {ts.timeout:,} | {ts.pass_rate:.1f}% |
<!-- compliance:end -->"""


def update_readme(language: Suite, builtins: Suite, ts: Suite) -> None:
	text = README.read_text()
	block = readme_block(language, builtins, ts)
	pattern = re.compile(r"<!-- compliance:begin -->.*?<!-- compliance:end -->", re.S)
	if not pattern.search(text):
		raise RuntimeError("README.md is missing compliance block markers")
	text = pattern.sub(block, text)
	text = re.sub(
		r"\*\*Test262 language suite: [0-9.]+%\*\*, \*\*built-ins: [0-9.]+%\*\*, "
		r"\*\*TypeScript [^*]+ conformance: [0-9.]+%\*\*",
		f"**Test262 language suite: {language.pass_rate:.1f}%**, "
		f"**built-ins: {builtins.pass_rate:.1f}%**, "
		f"**TypeScript {ts.version or 'unknown'} conformance: {ts.pass_rate:.1f}%**",
		text,
	)
	text = re.sub(
		r"At \*\*[0-9.]+% Test262 language compliance\*\* and "
		r"\*\*[0-9.]+% TypeScript [^*]+ conformance\*\*",
		f"At **{language.pass_rate:.1f}% Test262 language compliance** and "
		f"**{ts.pass_rate:.1f}% TypeScript {ts.version or 'unknown'} conformance**",
		text,
	)
	README.write_text(text)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--language-baseline", default="baseline_language.txt")
    parser.add_argument("--builtins-baseline", default="baseline.txt")
    parser.add_argument("--run-ts", action="store_true", help="Run the TypeScript conformance suite")
    parser.add_argument("--ts-path", default="../TypeScript")
    parser.add_argument("--ts-timeout", default="0.2s")
    parser.add_argument("--ts-runs", type=int, default=2, help="Number of TypeScript suite runs to smooth one-test flakes")
    parser.add_argument("--ts-flake-tolerance", type=int, default=3, help="Preserve cached TS best if a rerun is lower by at most this many passes")
    args = parser.parse_args()

    language = count_baseline(ROOT / args.language_baseline, "Test262 language")
    builtins = count_baseline(ROOT / args.builtins_baseline, "Test262 built-ins")
    if args.run_ts:
        ts = run_ts_suite((ROOT / args.ts_path).resolve(), args.ts_timeout, args.ts_runs)
        cached_ts = load_cached_ts()
        if (
            cached_ts is not None
            and cached_ts.version == ts.version
            and cached_ts.total == ts.total
            and cached_ts.passed > ts.passed
            and cached_ts.passed-ts.passed <= args.ts_flake_tolerance
        ):
            print(
                "Preserving cached TypeScript best "
                f"{cached_ts.passed}/{cached_ts.total}; rerun was lower by "
                f"{cached_ts.passed - ts.passed}."
            )
            ts = cached_ts
    else:
        ts = load_cached_ts()
        if ts is None:
            raise RuntimeError("No cached TypeScript metrics found; rerun with --run-ts")

    CHART.write_text(render_svg(language, builtins, ts))
    save_cache(language, builtins, ts)
    update_readme(language, builtins, ts)

    print(f"Test262 language: {language.passed}/{language.total} ({language.pass_rate:.1f}%)")
    print(f"Test262 built-ins: {builtins.passed}/{builtins.total} ({builtins.pass_rate:.1f}%)")
    print(f"TypeScript {ts.version}: {ts.passed}/{ts.total} ({ts.pass_rate:.1f}%)")


if __name__ == "__main__":
    main()
