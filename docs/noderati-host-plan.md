# Noderati host API plan

Tracker: [issue #77](https://github.com/nooga/paserati/issues/77).

Paserati stays the language runtime (parse → check → bytecode → VM). **noderati** is a sister Go module that hosts that runtime and supplies Node APIs. This document is the Paserati-side work so that sister repo does not have to fork `pkg/driver`.

Node built-ins (`fs`, `http`, `Buffer`, `require` implementation, `node_modules` algorithm, N-API) are **not** implemented here. They consume the seams below.

## What already works (do not rebuild)

| Seam | Where | How noderati uses it |
| --- | --- | --- |
| Extra globals | [`pkg/builtins/initializer.go`](../pkg/builtins/initializer.go) `BuiltinInitializer`; [`NewPaseratiWithInitializers`](../pkg/driver/driver.go) | `process`, `Buffer`, `setTimeout`, `global` |
| Go-backed ESM modules | [`DeclareModule`](../pkg/driver/native_module.go) / `ModuleBuilder` | `fs`, `path`, `os`, … |
| Native resolver priority | `NewNativeModuleResolver` priority `-100` vs filesystem `100` | Builtins win over `node_modules/fs` |
| Async I/O pattern | [`pkg/builtins/fetch_init.go`](../pkg/builtins/fetch_init.go) (`NewPendingPromise`, goroutine, `BeginExternalOp` / `EndExternalOp`) | `fs.promises`, `http` — copy this, do not use reflection `AsyncFunction` for I/O |
| Custom async runtime | [`pkg/runtime/async.go`](../pkg/runtime/async.go), [`VM.SetAsyncRuntime`](../pkg/vm/async_runtime.go), [`GetVM()`](../pkg/driver/driver.go) | Node event loop as a replacement `AsyncRuntime` |
| Script compile (Function) | [`CompileProgramAsScript`](../pkg/driver/driver.go) | Starting point for CJS wrapping, not a host “run this file” API yet |
| Skip types for `.js` | `SetSkipTypeCheck(true)` | Default for Node JS; keep types on for `.ts` |

A hello-world binary that only `DeclareModule("path", …)` could already compile. It cannot grow into a Node host: `moduleLoader` is unexported, so a sister module cannot install a `node_modules` resolver.

## Architecture

```
noderati cmd
    │
    ├─ NewPaseratiWithInitializers(standard + process/Buffer/timers)
    ├─ DeclareModule("fs" | "path" | …)
    ├─ DeclareModuleAlias("node:fs", "fs")     // same NativeModule, same ModuleRecord
    ├─ AddResolver(NodeResolver)               // bare specifiers, package.json
    └─ host event loop (custom AsyncRuntime)
            │
            ▼
     paserati driver (this repo)
            │
     lexer → parser → checker → compiler → VM
```

Two layers of Node API, on purpose:

1. **Globals** → `BuiltinInitializer` (same shape as [`ConsoleInitializer`](../pkg/builtins/console_init.go) / the existing [`ProcessInitializer`](../pkg/driver/process_init.go)).
2. **Importable builtins** → `DeclareModule`, then alias `node:*`.

Do **not** grow [`ProcessInitializer`](../pkg/driver/process_init.go) further in this repo. noderati owns a full `process`. The CLI stub can stay as argv wiring for `paserati` itself.

---

## Phase 0 — embed API (this branch)

**Goal:** a program outside `cmd/paserati` can register resolvers, alias `node:fs` → `fs`, and `import fs from "fs"`.

Unblocks starting the noderati repo.

### 0.1 `AddResolver` / `GetModuleLoader`

[`moduleLoader`](../pkg/driver/driver.go) is an unexported field. [`FileSystemResolver.CanResolve`](../pkg/modules/resolver_fs.go) is only `./`, `../`, and absolute paths. Bare `"lodash"` dies with `no resolver could handle specifier`.

Surface:

```go
func (p *Paserati) AddResolver(r modules.ModuleResolver)
func (p *Paserati) GetModuleLoader() modules.ModuleLoader
```

`AddResolver` is a thin wrap of [`moduleLoader.AddResolver`](../pkg/modules/loader.go) (re-sorts by `Priority()`, lower wins).

**Advice:** noderati’s Node resolver should sit between native (`-100`) and filesystem (`100`), e.g. priority `0`. Let native keep `fs`. Let the Node resolver handle bare specifiers *and* eventually relative paths that need `package.json` `"exports"` (it can beat the FS resolver by using a lower priority number). Do not teach the Paserati FS resolver Node rules.

### 0.2 Specifier aliases + one `ModuleRecord`

[`NativeModuleResolver.CanResolve`](../pkg/driver/native_module.go) is exact-name. Registering `"fs"` twice (once as `"node:fs"`) would init twice and break identity (`require('fs') === require('node:fs')` in Node).

Surface:

```go
func (p *Paserati) DeclareModuleAlias(alias, canonical string) error
```

Resolver maps both specifiers to the **same** `*NativeModule`. `Resolve` always returns `ResolvedPath = "native://" + canonicalName`.

The loader caches by **specifier**, so that alone is not enough. Phase 0 also keys the registry by `ResolvedPath`: the second specifier aliases the existing record. That is the same mechanism Node uses for `foo` vs `foo/index.js`, and noderati’s `node_modules` resolver will need it.

### 0.3 `ModuleBuilder.Default`

Today a TODO; Node does `import fs from "fs"`. Checker/compiler look up the `"default"` export ([`ModuleEnvironment.ResolveImportedType`](../pkg/checker/module_environment.go), [`compileImportDeclaration`](../pkg/compiler/compiler.go)).

- `Default(fn)` / `Default(primitive)` — named `"default"` export.
- `Default(nil)` — **namespace object of all named exports** (Node CJS interop for builtins). Call this **after** the named `Function`/`Const` registrations; it is applied when the builder returns.

**Advice:** for `fs`/`path`/`os`, use `Default(nil)`. For real I/O functions, still implement bodies with `vm.NewNativeFunction` + the fetch Promise pattern, not `ModuleBuilder.AsyncFunction`.

### Tests (Phase 0)

[`pkg/driver/host_embed_test.go`](../pkg/driver/host_embed_test.go) is the embed contract noderati should be able to copy:

- `AddResolver` + in-memory bare specifier
- `DeclareModule` + `DeclareModuleAlias` → **same record pointer**
- named import from `"fs"` and default import from `"node:fs"`
- `Default(nil)` namespace: `fs.readFileSync(...)` via default import

---

## Phase 1 — macrotasks and idle policy

**Why noderati stalls without this:** [`AsyncRuntime`](../pkg/runtime/async.go) is microtasks + “external op” wait. [`process.nextTick`](../pkg/driver/process_init.go) **calls the callback immediately**. Timers are on the bucketlist ([`docs/bucketlist.md`](bucketlist.md)). [`runAsModule`](../pkg/driver/driver.go) always `DrainMicrotasks()` then returns.

Node order: nextTick → microtasks → timers → I/O → setImmediate. Putting `setTimeout` on `ScheduleMicrotask` (via `BeginExternalOp` + sleep) will pass demos and fail real programs (`Promise.resolve().then` vs `setTimeout(0)`).

**Paserati work:**

- [x] Extend `AsyncRuntime` with nextTick / macrotask / timer hooks. `RunUntilIdle` stays microtask-only. Timer expiry goroutines only mark a timer due; callbacks run on the drain thread. `CancelTimer` drops both scheduled and already-due timers.
- [x] `VM.DrainUntilIdle()` and `runAsModule` drain until host idle (nextTick → microtasks → due timers → macrotasks → wait external / next timer). Top-level await uses the same queues. Hosts that replace `AsyncRuntime` still get this drain via the interface.
- [x] Do **not** put `setTimeout` in [`GetStandardInitializers`](../pkg/builtins/standard.go). Opt-in [`NewHostTimerInitializer`](../pkg/driver/host_timers.go) (`nextTick` / `setTimeout` / `clearTimeout`) is the reference host wiring noderati can copy or replace.

**Advice:** prefer replacing `AsyncRuntime` in noderati over adding a libuv clone here. Keep paserati’s default runtime for Test262 / smoke tests. Fetch already uses `BeginExternalOp`; timers hold the process alive via `HasPendingTimers`, not by pretending to be microtasks.

**Tests:** [`pkg/runtime/async_test.go`](../pkg/runtime/async_test.go), [`pkg/driver/host_idle_test.go`](../pkg/driver/host_idle_test.go) — Promise vs `setTimeout(0)`; `nextTick` before microtasks; drain waits for a timer; TLA + timer; `clearTimeout` after drain; standard builtins have no `setTimeout`.

---

## Phase 2 — script-mode execution (CJS seam)

**Why:** [`RunCode`](../pkg/driver/driver.go) always goes through `runAsModule`. Most of npm is still CommonJS. noderati should wrap:

```js
(function (exports, require, module, __filename, __dirname) {
  /* file body */
})
```

[`CompileProgramAsScript`](../pkg/driver/driver.go) exists for `Function()` (`import.meta` forbidden) but is not “run this path as a script with a filename”.

**Paserati work:**

- [x] Public `RunScript(source, filename)` and `RunOptions{Script: true, Filename}` compile as a Script (`ForceScriptMode`, no `EnableModuleMode`), set `SetCurrentModulePath(filename)`, and do not create a `ModuleRecord` or ESM exports.
- [x] Filename is the current path the host should pass through as `__filename` (and from which it derives `__dirname`) after wrapping the body.

**Advice:** implement `require()` in noderati as a native function that `LoadModule`s / wraps / caches `module.exports`. Do not teach the parser that `module.exports =` is ESM. Detection (`package.json` `"type"`, `.cjs`/`.mjs`, `--input-type`) is noderati’s.

**Tests:** [`pkg/driver/host_script_test.go`](../pkg/driver/host_script_test.go) — wrap a CJS snippet, `Call` it with an exports object, read `module.exports`; `__filename` matches the path passed in; no module record; `import.meta` rejected after a prior ESM `RunCode`.

---

## Phase 3 — `import.meta.url` as `file://`

[`OpLoadImportMeta`](../pkg/vm/vm.go) stores the raw module path. The comment already says a real environment would use `file://`. Node ESM (`new URL('./x', import.meta.url)`, `fileURLToPath`) depends on it.

**Paserati work:**

- [x] Emit `file://` URLs for filesystem modules (absolute path, proper slashes, encode).
- [x] Native modules can stay `native://fs` (or omit `.url` consumers).

**Advice:** this is a VM/driver change, not noderati. Do it before advertising ESM Node compat. Relative `import` from `import.meta.url` still goes through resolvers.

**Tests:** [`pkg/driver/host_import_meta_test.go`](../pkg/driver/host_import_meta_test.go) — `new URL('./foo.ts', import.meta.url).pathname` on a real file path (via `net/url`); native `native://fs` unchanged; eval `__code_module__` not abs-pathed into cwd; percent-encoding for spaces.

---

## Phase 4 — `AsyncFunction` actually returns a Promise

[`ModuleBuilder.AsyncFunction`](../pkg/driver/native_module.go) delegates to `Function` (sync). Fine for `path.join`; a lie for `fs.promises.readFile`.

**Advice:** noderati should **not wait** on this for I/O. Use the fetch pattern. Still fix the declarative API so hosts are not surprised.

**Tests:** `AsyncFunction` returning a Go `string` is thenable; `await` gets the string.

---

## Phase 5 — native module types are real

[`NativeModuleSource.generateSyntheticSource`](../pkg/driver/native_module.go) still emits placeholder `PI_SQUARED` / `square()`. Runtime skips parse via `IsNativeModule()`, and [`handleNativeModuleSource`](../pkg/modules/loader.go) already copies `GetTypeExports()` onto `record.Exports`. The placeholder is a footgun if anything ever *parses* native source.

**Paserati work:** generate declarations from `exports` (or stop implementing `Read()` as fake TS). Needed when noderati typechecks user TS that imports `fs`.

---

## Phase 6 — start noderati (other repo)

Once Phase 0 is on paserati `main` (or a `replace` directive):

1. `github.com/nooga/noderati` with `go.work` pointing at local paserati.
2. `cmd/noderati` — Node-shaped CLI: file, `-e`, `-p`, REPL, `process.argv = [execPath, script, …]`, exit 0/1 not sysexits 70.
3. `DeclareModule("path"|"os"|"util")` + aliases; own `process` initializer.
4. Then `fs` sync + `fs.promises` (fetch pattern), `Buffer`, timers (needs Phase 1), `events`/`stream`, then `require` (needs Phase 2).

N-API / `.node` files stay optional and CGO-tagged in noderati. Keep CGO out of paserati.

---

## Suggested PR slices

| Slice | Contents | Unblocks |
| --- | --- | --- |
| **0 (this branch)** | `AddResolver`, `GetModuleLoader`, `DeclareModuleAlias`, `Default`, resolved-path cache, embed tests | Sister repo can exist |
| 1 | `AsyncRuntime` macrotasks + idle policy | Honest `setTimeout` / `nextTick` |
| 2 | `RunScript` / CJS wrapper seam | `require()` |
| 3 | `file://` `import.meta.url` | Node ESM packages |
| 4–5 | `AsyncFunction`, synthetic d.ts | nicer host API, TS checking of builtins |

Do not mix Node `fs` into these PRs.

## Out of scope forever (in paserati)

- Implementing Node stdlib
- `node_modules` / `"exports"` maps (host resolver)
- Loading `.node` / N-API / V8 `Nan::`
- `--inspect`, `worker_threads`, `cluster`
