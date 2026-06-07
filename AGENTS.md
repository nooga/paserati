# AGENTS.md

This file provides guidance to Codex (and any other coding agent) when working in this repository.

## Project Overview

Paserati is a TypeScript/JavaScript runtime in pure Go. It parses + type-checks TypeScript and compiles **directly to bytecode** for a register VM — no JS transpilation step. Two deps (`regexp2`, `golang.org/x/text`), no CGO.

**Priorities**: correctness first (full ECMAScript 262 + TypeScript conformance), performance second (inline caches, shape-based objects, register allocation). The type checker can be disabled (`--no-typecheck`) to test pure JS semantics — this is how the Test262 driver runs.

## Commands

### Build and run

```bash
go build -o paserati ./cmd/paserati/
./paserati                                 # REPL
./paserati path/to/script.ts               # run file
./paserati -e 'expr'                       # eval expression
./paserati -bytecode script.ts             # dump bytecode
./paserati -cache-stats script.ts          # IC stats
./paserati --no-typecheck file.js          # pure-JS mode (Test262 repros)
```

Always rebuild before testing. `go build ./...` verifies every package compiles.

### Tests

```bash
go test ./tests/...                        # all tests
go test ./tests -run TestScripts           # smoke test — MUST stay green
go test ./tests -run "TestScripts/foo.ts"  # single script test
go test ./tests -v                         # verbose
```

### Test262

```bash
go build -o paserati-test262 ./cmd/paserati-test262

# Overview by subsuite with pass rates
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s

# Single test for debugging
./paserati-test262 -path ./test262 -subpath "language/expressions/addition" -pattern "S11.6.1_A3.1_T1.1.js"

# Diff current run against committed baselines (post-commit hook regenerates both)
./paserati-test262 -path ./test262 -subpath "language"  -timeout 0.2s -diff baseline_language.txt
./paserati-test262 -path ./test262 -subpath "built-ins" -timeout 0.2s -diff baseline.txt
```

Key flags: `-suite` (per-subsuite pass rates), `-filter` (skips legacy patterns like `with`), `-timeout` (per-test), `-subpath` (scope), `-pattern` (single file), `-diff <file>` (compare to baseline), `-dump <file>` (write current state).

Test262 itself is pinned to the SHA in `.test262-rev`; `./setup-test262.sh` clones / checks out that revision non-interactively.

### Failure clustering

```bash
go build -o paserati-analyze ./cmd/paserati-analyze/
./paserati-test262 -subpath language/expressions -json | ./paserati-analyze
```

Output groups failures by normalized error pattern. `[42] undefined is not an object` means 42 tests share that root cause — high-yield target.

## Smoke test format

`tests/scripts/*.ts` use these comments — parsed by `tests/scripts_test.go`:

```typescript
// expect: value                    // value of LAST STATEMENT, not console output
// expect_runtime_error: message    // substring match
// expect_compile_error: message    // substring match
```

The `// expect:` value is the result of the final expression. `console.log` output is **ignored** for comparison. Use `scratch/` (gitignored) for throwaway repros.

`TestScripts` is the smoke test: it must stay green. Fix the issue or mark `FIXME` — don't sweep.

## Development workflow

1. Make changes
2. `go test ./tests -run TestScripts` — verify smoke test stays green
3. `./paserati-test262 -path ./test262 -subpath "language" -timeout 0.2s -diff baseline_language.txt` — see Test262 impact
4. Commit (post-commit hook regenerates `baseline_language.txt` + `baseline.txt` for next iteration)

### Debugging Test262 failures

- Test262 tests JS semantics, **not TS types**. Always use `--no-typecheck` for repros.
- Type-checker fixes almost never move Test262 numbers. Focus on **compiler + VM**.
- Cluster errors with `paserati-analyze` before diving in — one root cause often unblocks dozens.

Common categories of fix:
- **Type coercion** — `ToFloat`/`ToString`/`ToBoolean` on `Value`; `vm.toPrimitive()` for `valueOf`/`toString`
- **Prototype chain** — built-ins must inherit from their prototype (e.g. `[] instanceof Array`)
- **Property access** — any value is a valid key (converts to string)
- **Error messages** — use `ValueType.String()` for human-readable type names

## Architecture

### Pipeline

```
lexer → parser → checker → compiler → vm
                     ↑
                  types
```

- `pkg/lexer/` — tokenization
- `pkg/parser/` — Pratt parser; AST in `ast.go`, per-feature files like `parse_class.go`
- `pkg/checker/` — TS type checking + narrowing; per-feature files like `class.go`, `narrowing.go`
- `pkg/compiler/` — bytecode gen + register allocation; per-feature `compile_*.go`
- `pkg/vm/` — register VM, inline caches, shape-based objects
- `pkg/types/` — type system (generics, conditional/mapped/template-literal types, `infer`)
- `pkg/builtins/` — prototype-registry built-ins (Array, String, Math, Proxy, Reflect, TypedArrays, …)
- `pkg/modules/` — ESM resolver and loader
- `pkg/driver/` — orchestrates pipeline; `Paserati` struct holds persistent REPL state
- `pkg/runtime/`, `pkg/source/`, `pkg/errors/` — supporting infra

### Adding a language feature

Touch the stages in order: lexer → parser → checker → compiler → vm → tests. **Create new `parse_*.go` / `compile_*.go` / feature-named checker files** instead of growing existing megafiles.

For a new operator:
1. Add token + precedence in lexer
2. Register infix/prefix parser
3. Add type-check case in checker's operator switch
4. Add compile case in compiler's operator switch
5. Add VM execution case (new opcode if needed)
6. Add test scripts in `tests/scripts/`

### VM essentials

Three object representations:
- `PlainObject` — shape-based, V8-style hidden classes (fast path)
- `DictObject` — hash-map for dynamic / many-key objects
- `ArrayObject` — specialized with `length` property

Property API: `HasOwn` / `GetOwn` / `SetOwn`. Arrays handle numeric indices specially.

Compiler uses register allocation with `defer`-based cleanup — each compile function frees its temps.

### Type system

Type checker uses widened types for compatibility and maintains separate environments for narrowing across control flow branches. Class types are represented as constructor function types.

### Debug flags

Each stage has a `const debug<Stage> = false` at the top of its main file (`pkg/{lexer,parser,checker,compiler}/<stage>.go`). Flip to `true` for tracing.

## Known gotchas

**Flaky in batch / green individually** — ignore in diff summaries:
- `language/statements/for-of/map.js`
- `language/statements/for-of/map-contract-expand.js`
- `language/statements/for-of/set-contract-expand.js`

**TS class properties** must be explicitly declared in the class body (TypeScript requirement, enforced by the checker).

**Object key coercion** — `undefined` as a property key becomes `"undefined"`, etc.

## Conventions

- Smoke test (`TestScripts`) must stay green. Fix it or mark `FIXME`.
- No `git -i` flags (no interactive rebase / `add -i` — agent tooling can't handle them).
- Don't prefix branches with `codex/` unless explicitly asked.
- Match TypeScript compiler output for type-error wording; match ECMAScript spec for runtime-error wording.
- Never sweep failures under the rug. Either fix or `FIXME`.

For implementation status by feature, see `docs/bucketlist.md`. For current Test262 pass rates, see `README.md` (auto-updated) or run with `-suite -filter`.
