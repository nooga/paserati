[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/nooga/paserati)

## PASERATI

![Paserati](paserati.png)

### _"Sir, it's no V8 but we're doing what we can"_

Paserati is an **experimental (mad scientist kind of experimental) TypeScript runtime**: it parses + type-checks TypeScript and compiles it **directly to bytecode** for a register VM. And then it executes the bytecode.

TypeScript is a superset of JavaScript, so if you can execute TypeScript, you’re building an **ES2025 runtime** too (by definition, plus a lot of pain).

### What’s under the hood

Under the hood: **TS/JS → bytecode → register VM**, with **inline caches**, **shape-based objects** (a.k.a. “hidden classes”-style), and a pluggable async executor with **microtask scheduling**.

Right now it prioritizes **correctness** over raw speed, but the architecture is designed for type-driven optimization later (specialization, monomorphization, unchecked fast paths).

### Wins

- **Test262 language suite: 92.0%** (see details below)
- **Native TS execution**: no `tsc`, no TS→JS transpilation
- **TCO**: tail call optimization (elite feature)
- **Shapes + ICs**: fast-ish property access without a JIT
- **Runtime type reflection**: `Paserati.reflect<T>()` (generate a type object / JSON Schema at runtime)
- **Small-ish footprint**:
  - **~16MB static binary** (unstripped, includes lexer/parser/checker/compiler/VM/builtins)
  - **~5MB BSS idle** (approx; depends on build/OS)
  - **Pure Go**, no CGO, no WASM blobs, **two simple dependencies** (`golang.org/x/text`, `github.com/dlclark/regexp2`)

### Weird flex but okay benchmarks

Paserati has a long way to go performance-wise, but it’s already at the point where it can **beat [dop251/goja](https://github.com/dop251/goja)** and **QJS** on a couple of simple microbenches.

Results from `hyperfine` (see `bench/hyperfine.sh`):

| Benchmark          | paserati (Mean) |     goja (Mean) |       QJS(Mean) |                                                            Relative |
| :----------------- | --------------: | --------------: | --------------: | ------------------------------------------------------------------: |
| `bench/bench.js`   | 3.924 ± 0.097 s | 5.207 ± 0.093 s | 4.945 ± 0.092 s | **paserati 1.33× faster than gojac**, **1.26× faster than quickjs** |
| `bench/objects.js` | 6.169 ± 0.092 s | 7.015 ± 0.151 s | 8.135 ± 0.130 s | **paserati 1.14× faster than gojac**, **1.32× faster than quickjs** |

Paserati also [runs V8 benchmarks](<https://ahaoboy.github.io/js-engine-benchmark/?kind=Time(s)&selectEngines=goja,paserati&sort=Time(s)>), beating Goja in several.

If your favorite pure Go JavaScript engine is reading this: _skill issue_.

### Examples

- **Runtime type reflection / JSON Schema**: `examples/reflect.ts`
- **Proxy + classes + async**: `examples/reactive.ts`
- **Async/await + generators**: `examples/async.ts`
- **Classes (private fields, inheritance, statics)**: `examples/classes.ts`
- **Typed recursion (Y combinator)**: `examples/ycomb.ts`
- **“Look ma, generics”**: `examples/generics.ts`

Examples may or may not work at every commit, but they should work at least once in a while.

### What it isn’t (non-goals)

- **A TypeScript build toolchain replacement**: see [microsoft/typescript-go](https://github.com/microsoft/typescript-go)
- **A JIT in Go**: I’ll stop just short of that (for now)
- **Perfect “legacy weirdness”**: modern ES is the target; some dusty corners (like `with`) are still incomplete

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

### Test262 results (language suite)

From a recent run:

```
language (TOTAL)             23410    21533     1852        0       25    92.0%    1m41.734s
```

Weak spots:

- `language/import/import-defer`: 14.5% (69)
- `language/module-code/namespace`: 52.8% (36)
- `language/statements/with`: 55.8% (181)
- `language/eval-code/indirect`: 62.3% (61)
- `language/expressions/yield`: 74.6% (63)
- `language/expressions/super`: 75.5% (94)
- `language/statements/for-in`: 78.3% (115)
- `language/types/reference`: 79.3% (29)
- `language/expressions/call`: 79.3% (92)
- `language/eval-code/direct`: 79.7% (286)

### Current status

The engine runs a lot of modern code today, but it’s still a moving target. It might crash on you sometimes, or refuse to run some valid code. _I'm working on it. Okay?_

Some big-ticket items that work reasonably well:

- **Async/await, TLA, Promises, microtasks** (incl. top-level await, async generators)
- **ESM modules** (plus dynamic `import()`, pluggable resolution)
- **Classes** (private fields, statics, inheritance)
- **(Async) Generators** (`yield`, `yield*`)
- **Modern operators** (`?.`, `??`, logical assignment)
- **Destructuring** (arrays/objects/rest/spread)
- **Built-ins** (Proxy/Reflect/Map/Set/TypedArrays/ArrayBuffer/RegExp/Symbol/BigInt)
- **Advanced types** (generics, conditional/mapped types, template literal types, `infer`)

Last time I checked it could run a pure TS library [date-fns](https://github.com/date-fns/date-fns) from source without any glaring issues. _Cough, not sure if it does so at every commit, but it did at the time of writing._

Known gaps:

- WeakMap/WeakSet (tbd)
- Decorators (planned)
- `eval` correctness (still bugged; shows up in `eval-code/*` subsuites)
- `with` statement semantics (incomplete)
- Some ASI edge cases (I think, eh)
- Import attributes (experimental ES feature)

See [docs/bucketlist.md](docs/bucketlist.md) for the exhaustive yet messy feature inventory.

### Contributing

Seriously, why would you want to contribute to this? _…But if you do, I'm both terrified and thrilled. PRs and issues are welcome._

### License

This project is licensed under the MIT License.

### AI disclaimer

This is a **one-person** project developed in my **free time** with the help of **AI**. It is also an experiment in large scale software engineering with AI, aimed at speedrunning a production-quality open source project.

Google Gemini 2.5/3.0 Pro and Claude Sonnet/Opus 4/4.5 wrote almost all the code so far under more or less careful direction and scrutiny - also known as "vibe coding but when you know what you're doing".

That fun sticker at the top of the README? It's made with GPT-4o's image generation.

---

_Remember: Pedal to the metal, or just pedal faster._
