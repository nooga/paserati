[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/nooga/paserati)

## PASERATI

![Paserati](paserati.png)

### _"Sir, it's no V8 but we're doing what we can"_

Paserati is an experimental TypeScript runtime, the mad scientist kind of experimental. It parses and type-checks TypeScript, compiles it straight to bytecode for a register VM, and runs that bytecode. There is no `tsc` and no transpile-to-JS step anywhere in the pipeline.

TypeScript is a superset of JavaScript, so a TypeScript runtime has to be an ESNext runtime too.

### What's under the hood

Source goes through a lexer, a parser, a type checker and a compiler, and comes out as bytecode for a register-based VM. Objects use shapes (V8 calls them hidden classes), property access goes through inline caches, and async code runs on a pluggable executor with a microtask queue.

Right now correctness wins over speed. The checker already knows the types, though, and the plan is to use them later for specialization, monomorphization and unchecked fast paths.

### Wins

- Test262 language suite: **98.6%**, built-ins: **91.8%**, TypeScript 6.0.3 conformance: **45.2%** exact / **67.1%** loose (details below)
- Runs TypeScript natively. No `tsc`, no TS-to-JS step.
- Tail call optimization. Elite feature.
- Shapes and inline caches give fast-ish property access without a JIT.
- `Paserati.reflect<T>()` turns a type into a runtime type object or a JSON Schema.
- The binary is about 26MB, or 18MB stripped with `-ldflags "-s -w"`. That covers the lexer, parser, checker, compiler, VM and every builtin.
- `paserati -e '1'` peaks at about 21MB RSS and starts in about 10ms.
- Pure Go. No CGO, no WASM blobs, and three dependencies: `golang.org/x/text`, `github.com/dlclark/regexp2` and `github.com/rivo/uniseg`.

Sizes and timings are from an Apple Silicon Mac (arm64, macOS) with Go 1.27.1.

### Weird flex but okay benchmarks

Paserati has a long way to go on performance. Still, it beats [dop251/goja](https://github.com/dop251/goja) and [modernc.org/quickjs](https://gitlab.com/cznic/quickjs), which is QuickJS machine-translated to pure Go, on a couple of simple microbenchmarks. Those are the fair comparison. V8 and C QuickJS play a different sport.

These numbers come from `hyperfine` via `bench/hyperfine.sh`, on Apple Silicon in October 2026, with Paserati running under `--no-typecheck`.

| Benchmark          | paserati (mean) |     goja (mean) | QuickJS/modernc (mean) |                                    Relative |
| :----------------- | --------------: | --------------: | ---------------------: | ------------------------------------------: |
| `bench/bench.js`   | 2.404 ± 0.041 s | 5.175 ± 0.034 s |        4.905 ± 0.046 s | 2.15× faster than goja, 2.04× than QuickJS |
| `bench/objects.js` | 5.286 ± 0.036 s | 6.957 ± 0.052 s |        7.979 ± 0.041 s | 1.32× faster than goja, 1.51× than QuickJS |

Paserati also [runs the V8 benchmarks](<https://ahaoboy.github.io/js-engine-benchmark/?kind=Time(s)&selectEngines=goja,paserati&sort=Time(s)>) and beats goja on several of them.

If your favorite pure Go JavaScript engine is reading this, _skill issue_.

### Examples

- `examples/reflect.ts` turns types into runtime objects and JSON Schema
- `examples/reactive.ts` mixes Proxy, classes and async
- `examples/async.ts` covers async/await and generators
- `examples/classes.ts` shows private fields, inheritance and statics
- `examples/ycomb.ts` is a typed Y combinator
- `examples/generics.ts`, look ma, generics

Examples may or may not work at every commit. They should work at least once in a while.

### Usage

```bash
# Build
go build -o paserati ./cmd/paserati/

# Run the REPL
./paserati

# Run a snippet
./paserati -e 'console.log("hello from paserati")'

# Execute a script
./paserati path/to/script.ts

# Run the test suite
go test ./tests/...
```

### Conformance

<!-- compliance:begin -->
![Conformance snapshot](docs/compliance.svg)

| Suite | Passed | Failed | Skipped | Timeouts | Pass rate |
| :-- | --: | --: | --: | --: | --: |
| Test262 language | 23,186/23,523 | 337 | 0 | 0 | 98.6% |
| Test262 built-ins | 21,394/23,294 | 1,900 | 0 | 0 | 91.8% |
| TypeScript 6.0.3 conformance (exact) | 3,223/7,127 | 2,496 | 1,408 | 0 | 45.2% |
| TypeScript 6.0.3 conformance (loose) | 4,779/7,127 | 940 | 1,408 | 0 | 67.1% |
<!-- compliance:end -->

The TypeScript numbers cover every test in TypeScript 6.0.3's `tests/cases/conformance`. A test that declares several compiler-option variants counts once per variant. A test passes `exact` when Paserati reports the same error codes on the same lines as `tsc`, with nothing missing and nothing extra. `loose` only asks that we raised some error where `tsc` raised one, so it flatters us. The runner can't do multi-file or `.tsx` tests yet, and those skips count as failures. Exact is the number to trust.

`paserati-testtsc` produces the TypeScript figures by comparing our diagnostics with the reference baselines from the TypeScript 6.0.3 release (tag `v6.0.3`). The Test262 figures come from baseline snapshots of the checked-out ECMA-262 test suite.

### Current status

Paserati is past the toy project phase. It runs real TypeScript libraries like [date-fns](https://github.com/date-fns/date-fns) straight from source.

With [noderati](https://github.com/nooga/noderati), a Node-shaped host built on Paserati, it runs unmodified npm packages. That includes `tsc` itself, eslint with typescript-eslint, esbuild's JS API, marked, ajv, bcryptjs, express routing and vitest test runs. It also runs the [pi](https://www.npmjs.com/package/@earendil-works/pi-coding-agent) coding agent.

### Contributing

Seriously, why would you want to contribute to this? _But if you do, I'm both terrified and thrilled. PRs and issues are welcome._

### License

MIT.

### AI disclaimer

This is a one-person project that I work on in my free time, with a lot of help from AI. It's also an experiment in large-scale software engineering with AI. The goal is to speedrun a production-quality open source project.

Google Gemini 2.5/3.0 Pro, Claude Sonnet/Opus 4 through 5.5 and GPT 5.5 wrote almost all of the code, under more or less careful direction and scrutiny. Call it vibe coding, but the kind where you know what you're doing.

GPT-4o's image generation made the sticker at the top.

---

_Remember: Pedal to the metal, or just pedal faster._
