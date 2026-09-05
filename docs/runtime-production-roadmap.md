# Production compiler and runtime roadmap

Status: proposed implementation plan. Prepared 2026-09-05. This document changes
no runtime behavior and implements none of the proposed tools or flags.

Paserati's objective is a conformant, production-quality JavaScript/TypeScript
runtime in Go, with excellent performance without a native JIT. The immediate
strategy is to establish trustworthy correctness measurements, repair observable
optimization and lifetime defects, and then reduce executed work, property-access
cost, and memory traffic. Keep the direct TypeScript-to-bytecode pipeline.

The September 2026 audit found substantial engineering and competitive performance
against Goja. It did not establish production readiness or leadership among all
interpreters. Go does not prevent most of the improvements below. It does impose
constraints on value representation, collection isolation, and control over
generated machine code. Those constraints should guide experiments, not substitute
for measurements.

This plan is intended to be split into small implementation PRs. Work-item IDs
are stable references; their ordering expresses dependencies rather than dates.
The acceptance criteria are proposed requirements. Performance thresholds and
time budgets are initial policy choices to calibrate, not measured guarantees.

## Navigation

- [Evidence and scope](#evidence-and-scope)
- [Architectural commitments](#architectural-commitments)
- [Delivery sequence](#delivery-sequence)
- [A: correctness and optimization contracts](#a-correctness-and-optimization-contracts)
- [B: lifetime and representation](#b-lifetime-and-representation)
- [C: compiler optimization](#c-compiler-optimization)
- [D: property access and execution](#d-property-access-and-execution)
- [E: production embedding](#e-production-embedding)
- [F: development benchmarking infrastructure](#f-development-benchmarking-infrastructure)
- [Implementation and release gates](#implementation-and-release-gates)
- [Appendix: reproduction recipes](#appendix-reproduction-recipes)
- [References](#references)

## Evidence and scope

The audit benchmarked clean commit
[`64073a2956993079cb35209cb360397d9ba0d1f3`][audit-commit]. The source was later
rechecked at
[`2aa3681512eb69722f0322fce83cf56405afdc85`][plan-commit], the base of this plan.
That intervening change concerned inferred function-name bindings. A rebuild,
TestScripts, and the prototype, iterator, rest-array, switch, and constant-pool
source reproductions were repeated there. Performance was not remeasured at the
newer revision.

The audit examined compiler lowering and register allocation, bytecode, values,
objects, caches, calls, iterators, captures, suspension, and measurement tools.
It was not an exhaustive security review, a full TypeScript-checker audit, or a
proof of ECMAScript conformance. Revalidate findings against the implementation
revision before fixing them; a subsequent PR may already address one.

### Observations that determine priorities

| ID | Audit observation | Evidence class | Work |
| --- | --- | --- | --- |
| A01 | Six deliberately failing Test262 runner fixtures were reported as passing: missing/wrong negative errors, async completion failures, omitted strict variant | Reproduced runner behavior | A1 |
| A02 | After warming `leaf.x`, adding `middle.x` in the prototype chain still returned the old ancestor value | Reproduced source behavior | A2, D1 |
| A03 | Array iteration/destructuring read raw storage instead of an indexed getter; exhausted iterators resumed after growth | Reproduced source behavior | A3 |
| A04 | Separate calls returning an empty rest parameter returned the same mutable array | Reproduced source behavior | A4 |
| A05 | 65,538 distinct string assignments silently wrapped a constant index and returned the wrong string | Reproduced source behavior | A5 |
| A06 | Entering a later switch case read an uninitialized lexical binding, including a stale value from a previous invocation | Reproduced source behavior | A6 |
| A07 | An isolated function's large array stayed live after return and Reset; diagnostic clearing of inactive registers/frames released it | Reproduced embedding probe | B1 |
| A08 | Dropping 50,000 objects with unique property names left about 34 MB retained by global shape transitions; explicit cache clearing released it | Reproduced embedding probe | B2 |
| A09 | Independent register allocators raced on a global diagnostic ID | Isolated race-detector reproduction; not evidence of VM corruption | A7 |
| A10 | Reusing a hand-assembled chunk after a TDZ check rewrote only the opcode, leaving its operand to be decoded as code | Reproduced bytecode behavior; source-generated reachability unproven | A6, A8 |
| A11 | `return 1 + 2 * 3` emitted three constant loads, multiply, and add | Inspected emitted bytecode | C1, C2 |
| A12 | Object profile: property get about 36% inclusive CPU, IC lookup about 12%, slow metadata resolution about 7% | One workload's CPU profile; inclusive values overlap | D1, D2 |

The audit's existing `go build ./...`, `go test ./...`, and targeted package race
tests passed. The additional probes found failures those checks did not cover.
Passing existing tests is necessary and does not discharge the findings above.
Absolute Test262 pass percentages must be regenerated after A1; existing snapshots
are useful historical artifacts but cannot establish conformance.

### Historical performance, with limits

Host: Apple M1 Pro, darwin/arm64. Paserati and Goja: Go 1.26.0. Goja revision:
`f87b40ad7341` (2026-09-03). Original C QuickJS: 2026-06-04, default Makefile,
clang `-O2`. V8: Node 26.3.0 with `--jitless`. Paserati: `--no-typecheck`.

Median whole-process seconds, three sequential samples per engine with rotated
engine order, including startup and compilation; all checksums agreed:

| Workload | Paserati | C QuickJS | Goja | V8 without JIT |
| --- | ---: | ---: | ---: | ---: |
| `bench/bench.js` | 2.371 | 0.810 | 4.986 | 0.736 |
| `bench/objects.js` | 5.279 | 1.786 | 6.587 | 0.887 |

Ten V8 v7 workloads used original setup/checks/teardown, one untimed warmup, and
at least 500 ms of repeated execution in each of three fresh processes per
engine. Median milliseconds per iteration, excluding compilation and setup:

| Workload | Paserati | C QuickJS | Goja | V8 without JIT |
| --- | ---: | ---: | ---: | ---: |
| Richards | 11.409 | 2.656 | 10.521 | 2.062 |
| DeltaBlue | 16.258 | 4.827 | 16.645 | 4.175 |
| Encrypt | 13.514 | 4.365 | 29.882 | 3.704 |
| Decrypt | 253.000 | 81.286 | 554.000 | 70.125 |
| RayTrace | 109.600 | 23.773 | 159.250 | 12.675 |
| Earley | 11.814 | 4.587 | 20.160 | 1.898 |
| Boyer | 252.000 | 67.000 | 343.500 | 35.714 |
| RegExp | 156.000 | 168.000 | 270.500 | 23.091 |
| Splay | 3.906 | 1.347 | 5.652 | 0.882 |
| NavierStokes | 170.000 | 52.200 | 468.500 | 59.333 |

The geometric mean of workload time ratios put Paserati at approximately 1.60
times Goja's speed, 2.98 times C QuickJS's execution time, and 4.97 times
JIT-disabled V8's execution time. These are the audit's customized samples, not
an official V8 v7 score, confidence intervals, or a production-latency study.
All 120 process runs across both groups completed and passed workload checks.
Do not use these three-sample results as a regression baseline or promise that
an optimization will recover any particular fraction of the gap.

The local `qjs` used by the older repository comparison identified itself as
the `modernc.org/quickjs` Go translation. Its historical label must not silently
be interpreted as the original C engine. Future manifests must distinguish the
implementations and record executable provenance. [QuickJS documentation][quickjs]
and [V8's JIT-less explanation][v8-jitless] define the intended peer modes.

Raw audit files lived in an ignored scratch directory and are not dependencies
of this document. The tables, source references, and reproduction recipes here
preserve the actionable observations. F1 must create durable raw artifacts for
future measurements; it must not manufacture missing historical provenance.

## Architectural commitments

Preserve the useful foundations: direct TypeScript compilation, runtime type
reflection, register bytecode, indexed globals, cheap ordinary call frames,
shape transitions, GC-visible string storage, WTF-8/UTF-16 semantics, per-frame
capture ownership, and explicit suspension handling. These are practical
strengths. Reflection and the integrated Go/TypeScript execution pipeline are
product differentiators; this plan makes no claim of a new compiler algorithm.

The optimizer's contract is ECMAScript behavior, including observable allocation
identity, coercion order, getters, proxies, exceptions, and iterator closing.
TypeScript annotations may supply hints, but assertions, `any`, host values,
mutation, and external callers require runtime guards or proven boundaries.
Performance improvements must retain the supported language semantics.

Go is suitable for the planned compiler and most memory-layout work. Allocation
rate, retained graphs, and scannable pointer storage are design inputs under
Paserati's control. GC tuning trades memory for collection work; it does not
repair live references left in dead VM slots. [Go GC guide][go-gc]

Keep these constraints explicit:

- A compact `Value` must retain real, GC-visible references. Encoding the sole
  Go object address in a `uint64` or `uintptr` loses pointer semantics.
  [Go pointer rules][go-unsafe]
- Register addresses used by open upvalues must remain valid. Growing a slice
  does not guarantee its backing array stays in place; use stable blocks or
  change captures to checked logical locations.
- `GOMEMLIMIT`/`SetMemoryLimit` controls a soft runtime-wide limit, not exact
  per-VM memory ownership or an OS RSS cap. [Runtime memory-limit API][go-memory]
- Pure Go does not expose precise control over interpreter register assignment
  and handler calling conventions. Measure generated code before assuming a
  dispatch technique translates from C or assembly.
- A custom guest heap with integer handles is possible, but requires its own
  roots, tracing/reclamation, and host-reference integration. Defer it until
  measured residual costs justify a separate design review.

## Delivery sequence

| Phase | Deliverable | Dependencies | Exit evidence |
| --- | --- | --- | --- |
| 0 | F1/F2: bounded benchmark capture and validated workload identities | Existing tools | A/A report, hard deadline tests, raw samples, complete coverage accounting |
| 1 | A1–A8: trustworthy conformance and repaired optimization contracts | Can proceed alongside phase 0 | Reproductions fixed; harness adversarial fixtures and bytecode verification pass |
| 2 | B1/B2/B3: bounded lifetime, shape policy, compact metadata | Affected A contracts; F1 | Retained-heap plateaus, unchanged observable behavior, short A/B results |
| 3 | C1/C2 and D1/D2: cheaper compiler output and property access | A8, F2/F3; A2 for prototype caches | Representative workload gains and guard-failure tests |
| 4 | B4/B5, C3/C4, D3/D4: representation and specialization experiments | Stable earlier invariants | Measured benefit exceeds cost across required workload families |
| 5 | E1/E2 and F4/F5: production operation and sustained evidence | Starts early; completes before production declaration | Concurrency/latency/memory runs, documented compatibility and resource contracts |

Phases are not large batches to merge together. For example, fix empty rest
identity before optimizing calls; fix dead-slot clearing before replacing register
storage. A correctness repair may increase allocations or execution time. Record
that cost and establish the correct implementation as the new baseline.

## A: correctness and optimization contracts

### A1. Make the conformance runner authoritative

Start in [the Test262 runner][src-test262] and the shared result model in
[`pkg/test262`](../pkg/test262). Replace the current success heuristic with an
explicit execution result: variant, terminal status, observed error phase/type,
async completion, timeout, and diagnostic. Infrastructure failure is a separate
status from a JavaScript exception.

Expand test metadata into the required strict/non-strict/module/raw execution
variants. Negative tests require an error with the declared phase and type;
normal completion is failure. Async execution must consume the harness completion
signal and treat missing completion, failure completion, and timeout correctly.
Preserve include order and module-fixture handling. These rules come from
[Test262's interpretation contract][test262-rules].

Prefer structured runner/VM outcomes over parsing human-readable console text.
Where the harness uses `print`, bind the completion protocol at that interface
and preserve its diagnostic separately. Map parser/early-error, module-resolution,
and execution errors explicitly. Internal Go panics must never satisfy negative
tests. Ensure timed-out workers terminate before reporting completion; cancellation
must not leave background interpreters consuming CPU during later tests.

Give each result a stable key including corpus revision, relative test path, and
execution variant. Report passed/failed/skipped/timed-out/infrastructure-error
counts separately, with both file and variant denominators when useful. Version
baselines when identities or interpretation policy change. Keep legacy snapshots
readable but mark them incompatible with the corrected policy.

**Acceptance:** the six audit fixtures fail; paired positive controls pass;
wrong-phase errors and injected internal panics cannot pass; async failure wins
over apparent successful completion. Add metadata and worker-lifecycle tests.
Regenerate pinned language/built-in baselines with explicit feature exclusions,
and update README figures from the same output. Do not call the expected drop in
reported conformance a runtime regression. This is a prerequisite for using
Test262 correctness to qualify a performance result.

### A2. Specify property-cache validity

The default [property-get path][src-getprop] validates the receiver/holder too
narrowly for inherited hits. Correctness requires that the cached property still
resolves through the same relevant chain, including absence of nearer shadowing
properties and the meaning of the descriptor.

First repair the cache conservatively: record and validate each ordinary
intermediate object's identity/prototype link and shape/version as required by
the representation, or fall back to generic lookup. The receiver's shape alone
is not sufficient unless shape identity also proves the prototype relationship.
Reject exotic/proxy cases unless their semantics are explicitly supported.
Data writes at an unchanged slot should load the current slot value, not cache
the old value. Accessor calls must receive the original receiver.

Centralize invalidation for adding/deleting/redefining properties and replacing
prototypes. Write down which mutation paths participate, including Reflect,
Object.defineProperty, freeze/seal, class initialization, and host APIs. Optimize
the guard only after the conservative version is correct; D1 compares chain
guards against validity cells or epochs.

**Acceptance:** warm/mutate/read tests at every chain depth; replacement with an
equally shaped prototype; missing-to-present and data-to-accessor transitions;
deletion, proxy boundaries, symbols, and receiver-sensitive getters. Compare
fresh, warm, and forced-generic execution. Measure own-property and inherited
hits separately so fixing one does not conceal slowing the other.

### A3. Establish an array-iteration fast-path contract

[Builtin iterator stepping][src-iterator] may read packed storage only while
that read is equivalent to the required indexed property operation. Iterator
identity is necessary for some specializations and is not proof that indexed
reads lack accessors or inherited effects. Preserve dynamic length observation
while iteration is live and permanently record completion once it is exhausted.
[Array iterator semantics][spec-array-iterator]

Begin with generic indexed access for cases the runtime cannot cheaply prove
safe. Introduce a guard for ordinary dense own data elements, their attributes,
and relevant prototype state. Recheck after execution can call guest code. A
hole must not bypass an inherited getter. Cache builtin identities only with
invalidation for mutations to the iterator method/prototype.

Use one state transition implementation for `.next()`, for-of, and specialized
destructuring where their operations agree. Keep each construct's iterator-close
and abrupt-completion rules explicit instead of sharing a path that omits them.

**Acceptance:** indexed getters before and during iteration; holes and inherited
indices; length shrink/growth; iterator exhaustion followed by growth; custom
iterators; `.next()` interleaved with for-of; destructuring and early break;
throwing getters and iterator closing. Pair a packed-array benchmark with a
mutation-heavy fallback benchmark.

### A4. Restore fresh rest-parameter identity

Remove reuse of the mutable empty-rest singleton in [ordinary calls][src-call],
spread construction, tail calls, and the additional call paths in
[`vm.go`][src-vm]. Initially allocate a fresh empty array object whenever a rest
parameter is observable. An immutable shared zero-length backing store can still
be used if each array object has distinct identity and independent metadata.

Do not replace the singleton with a pool of objects that remain observable.
Escape-based delayed materialization belongs in a later compiler experiment.
Audit `arguments`, closures, constructors, and host reentry for comparable
identity shortcuts.

**Acceptance:** two returned rest arrays are unequal; mutation, descriptors, and
prototype changes do not leak between invocations. Cover direct, spread, bound,
constructor, async/generator, and tail-call routes. Record any allocation increase
as the cost of the semantic repair, then optimize internal argument movement in D3.

### A5. Make operand limits explicit and checked

All paths through [`Chunk.AddConstant`][src-bytecode] must check representability
before narrowing an index, including type-specific deduplication caches. A pool
with 65,536 entries can address indices 0 through 65,535; a duplicate must remain
usable at capacity, while a new unrepresentable entry must fail predictably.

First return a structured compilation error through callers. Do not use a Go
panic or silently wrap. Then inventory every narrow field: registers, arguments,
globals, spills, captures, branch displacements, and exception offsets. Introduce
checked emitter helpers so each lowering path does not repeat boundary logic.
Test positive/negative zero and NaN behavior in numeric constant deduplication;
host map equality must not define JavaScript constant equivalence accidentally.

A separate PR can add wide forms or chunk splitting if generated bundles require
them. Specify the encoding, mixed-width decoding, disassembly, and verification
together. An early compilation error is preferable to an incorrect result until
that feature exists.

**Acceptance:** direct pool tests around 65,535/65,536/65,537 entries, duplicate
insertion at capacity, and generated-source boundary cases; corresponding tests
for each operand width. No process crash or successful miscompilation.

### A6. Repair lexical initialization and unsafe bytecode rewriting

Unify [switch lexical binding setup][src-switch] with the block-scope contract:
allocate bindings in their lexical environment, initialize them to the TDZ
sentinel before control can enter a case, and check reads until initialization is
proven on that control-flow path. Include class bindings and closures. Repeated
calls must not expose a previous activation's register contents.

Remove the TDZ check's opcode-only self-rewrite in [`vm.go`][src-vm]. Replacing a
multi-byte instruction with a one-byte NOP changes instruction boundaries. More
fundamentally, one activation's successful check does not establish initialization
in future activations. Prefer immutable semantic bytecode and side metadata for
caches. Any later quickening must preserve decoding and guard validity.

**Acceptance:** direct entry at every switch case; fallthrough; nested blocks;
closures; class declarations; repeat execution with different paths; eval; and
generator suspension before initialization. Reuse the hand-assembled chunk from
the audit to test bytecode stability, while keeping its source reachability caveat.

### A7. Define concurrency ownership and remove the allocator race

Make the allocator diagnostic ID local, remove it when unnecessary, or use an
atomic counter. Add a race test constructing independent allocators concurrently.
Expand that ownership audit to global shapes, prototype registries, cache flags,
and UTF-16 cache state. Existing benchmark tests modify process-global cache
configuration, so separate their processes or restore state reliably.

**Acceptance:** race tests for independent compilation and independent VMs, plus
a documented single-owner policy for an individual VM unless broader concurrency
is explicitly supported. No global mutation during another VM's execution merely
to configure a benchmark. Do not infer a concurrent VM corruption bug from A09.

### A8. Add bytecode verification and a generic reference mode

Define one instruction-description table for widths, operands, register uses/defs,
control-flow effects, and cache-site metadata. Use it in the verifier and
disassembler, and eventually in C1. Verify instruction boundaries before branch
targets; check constant kinds, register/spill/global indices, closure captures,
exception intervals/handlers, and frame requirements. Verify recursive nested
chunks and instruction-specific operand relationships.

Enable verification on compiler output in tests and debug builds, and at any
public boundary accepting untrusted or externally assembled chunks. Explicitly
define whether that boundary is supported; verification alone does not make
arbitrary host pointers in constants safe. Production compiler-generated chunks
may use a documented trust boundary once coverage is established.

Add a test configuration that disables property/iteration/call specializations
and uses generic semantic operations. Existing prototype-cache flags are not an
all-optimizations-off reference mode. Compare results, exception types, side-effect
logs, and identity relationships between modes. Keep separate VMs/realms so one
run cannot mutate the other's inputs. Add seeded differential generation and
bounded fuzzing of emit/decode/verify and suspension boundaries.

**Acceptance:** truncated instructions, mid-operand branches, bad captures,
invalid exception tables, and boundary-sized functions are rejected or correctly
executed. Fuzz crashes become minimized regression cases. Generic/optimized
parity cases run cheaply in ordinary CI; extended fuzz campaigns run separately.

## B: lifetime and representation

### B1. Release dead register and frame references

Treat frame exit as an ownership transfer with a defined order: preserve return
or pending completion values; close captures or transfer suspended state; move
any live arguments/results; clear the now-dead register range and frame fields;
then make the storage reusable. Apply this to normal return, exception unwind,
tail-call replacement, cancellation, generator close, and Reset. Consolidate
small lifecycle helpers without forcing a Go call into every opcode.

Inventory reference-bearing fields beyond registers: saved arguments, closures,
pending finally values, iterators, error slices, and suspended state. Slicing a
container to zero length does not clear references in its backing allocation.
Track touched high-water ranges if Reset needs to clean already-popped storage;
avoid wiping the entire maximum stack on every ordinary return.

The audit's isolated two-million-element array probe measured approximately
28.1 MB after VM creation, 78.6 MB after return and GC, still 78.6 MB after Reset,
and 28.1 MB after diagnostic clearing. These are decimal MB and demonstrate
retention in that probe, not a target for every host or proof of complete VM
disposal. [Frame and register ownership source][src-vm]

**Acceptance:** repeated batches of allocate/use/discard requests reach a bounded
post-GC plateau; a surviving closure retains only the state it should; suspended
generators/async functions resume correctly after other calls reuse stack space.
Use large signal sizes and isolated processes for heap checks, with a justified
tolerance for runtime noise. Pair these with call throughput and allocation
measurements. Zero-allocation calls are desirable only where semantics permit.

### B2. Bound shape and other cache lifetimes

The [global root shape][src-shape] retains transition trees. Define an ownership
domain, preferably a VM/realm-group or explicit embedding runtime, and ensure all
object-creation paths use it. This touches helpers currently called without a VM;
do not move one cache while leaving hidden global roots elsewhere.

Recommended first implementation: bounded transition caches within that domain,
stable immutable shapes for existing objects, and dictionary promotion for objects
with high key churn. Evict transition-cache edges, never mutate live object layout
or reuse shape identity unsafely. An evicted shape may remain alive through an
object or IC, which is correct; separately bound caches that retain it. Cache
eviction must tolerate later recreation of an equivalent but distinct shape.

Measure shape count, transition-edge count, retained bytes, dictionary promotions,
and IC misses. Choose promotion thresholds from stable-layout and adversarial
unique-key workloads. Avoid unbounded global interning and manual global flushing
as the production policy. Weak references are an optional later implementation
choice, not a prerequisite for bounded caches.

Apply the same accounting to the UTF-16 identity cache, compiled regex cache,
module caches, and builtin/prototype registries. A bounded entry count can still
retain a large string or graph; add retained-size tests. Preserve the useful
linear-path shape sharing and memoized attribute transitions.

**Acceptance:** the 50,000-unique-key workload is bounded by documented cache
policy; repeated batches plateau; dropping a VM releases its domain unless host
objects intentionally retain it. Stable-shape programs retain useful hit rates.
Independent instances pass race tests and cannot flush each other's state.

### B3. Compact bytecode metadata and IC storage

The audited 64-bit build used 24-byte Values, 144-byte PlainObjects, 112-byte
ArrayObjects, 112-byte Shapes, 312-byte property ICs, and 304-byte CallFrames.
These are shallow sizes, not total retained cost or ABI commitments. `Chunk.Lines`
stores an eight-byte integer per byte of bytecode; a chunk using property caches
allocates another pointer per byte of bytecode. [Chunk layout][src-bytecode],
[cache-site allocation][src-ic-site], [cache layout][src-cache]

Assign dense cache-site IDs during emission and encode a checked ID at cacheable
instructions. Keep a compact monomorphic entry and allocate polymorphic overflow
only when needed. Compare the additional operand bytes/decoding cost with saved
table space; do not claim a memory win solely from struct size.

Replace per-byte source lines with runs of `(pcDelta, sourcePositionDelta)` or a
similarly compact table. Add sparse checkpoints if random PC lookup needs them.
Preserve filename, line/column behavior, exception attribution, and debugger
stepping. Cold error reporting can tolerate a different lookup tradeoff from
hot instruction execution. Release emission-only constant-dedup maps after chunk
finalization if no supported incremental path still needs them.

Specify cache ownership when a compiled chunk is reused across VMs/realms:
semantic bytecode may be shared, but mutable ICs and realm-specific assumptions
must be per execution owner or safely synchronized and fully guarded. The existing
per-chunk cache is not automatically a concurrency contract.

**Acceptance:** metadata-byte reports for small scripts and large bundles; exact
diagnostic/stack-trace behavior; verifier checks for site IDs and source maps;
compile/execute A/B results; multi-VM chunk-reuse tests for the documented API.

### B4. Replace eager maximum register allocation with stable segments

The default stack reserves `256 * 4096` Values, about 24 MiB before frames and
builtins. Prototype a directory of lazily allocated fixed-capacity blocks. A
frame's register window must remain contiguous within one block; an oversized
window gets a suitable dedicated block. The directory can grow without moving
the blocks referenced by open upvalues. Frame records also need stable storage
where pointers to them survive calls.

Represent stack marks as segment/offset/length and make push/pop ownership
explicit. Integrate tail-call resizing, exception unwinding, and suspension. Keep
only a bounded reserve of empty segments after a deep request; interior pointers
can otherwise retain an entire block. Preserve the existing relocation contract
for suspended captures rather than accidentally retaining the whole active stack.

Compare this design with captures addressed by logical slots. The latter can
permit relocation but adds indirection and more lookup invariants. Select using
cold-start, shallow-call, deep-recursion, and closure-heavy measurements. Do not
replace stable storage with unchecked slice growth.

**Acceptance:** substantially lower initialized-VM memory measured on declared
hosts; no unacceptable call-path regression under F3; deep recursion and repeated
growth/shrink; closures across segment boundaries; async/generator suspension;
bounded reserve after return. Define stack-limit behavior independently of the
chosen segment size. B1 must be correct first.

### B5. Experiment with compact Values and specialized elements

Prototype a 16-byte Value with an encoded tag/number/payload word and a separate
GC-visible pointer word. Write the bit-level representation contract first:
numeric special values, negative zero, integer range, non-number payload range,
string lengths, nil-pointer cases, and conversions. Canonicalizing number NaNs
may provide tag space; it requires explicit typed-array and bit-observation tests.
Keep actual references in pointer-typed storage. [Pointer requirements][go-unsafe]

Do not put arbitrary numbers in a field scanned as a pointer or retain Go heap
addresses solely inside integer tags. A compact integer-handle design requires a
separate lifetime/root design. A 24-to-16-byte change reduces Value storage by
one third; it does not imply a one-third runtime improvement and still leaves a
pointer word to scan in ordinary Value arrays.

Separately evaluate dense numeric elements stored in pointer-free `[]float64` or
integer buffers. Define transitions to generic storage for mixed values, holes,
descriptors, and unsupported indexed semantics. Preserve negative zero, NaN,
length limits, accessor/prototype effects, and allocation failure behavior.
Start with one representation and one promotion direction before optimizing
transitions. Literal shape templates can similarly avoid repeated construction
work, but each object and mutable property storage must remain independent.

**Acceptance:** full affected semantics plus unsafe/checkptr/race validation;
allocation/heap/scan measurements; arithmetic, mixed-value, call, string, and
object workloads on arm64 and amd64. Keep the current representation available
for A/B experiments until the new design demonstrates broad value. A custom
collector or a language rewrite is outside this work item.

## C: compiler optimization

### C1. Introduce a small control-flow and effects layer

Build on the instruction table from A8. Decode emitted code into basic blocks
with symbolic branch targets, or introduce equivalent blocks immediately before
encoding. Start with a per-function representation, not a whole-program rewrite.
Record source positions, register uses/defs, exception edges, and conservative
effects: may throw, call guest code, allocate observable identity, read/write
bindings or properties, and suspend. Unknown operations have maximal effects.

Initially support only local transformations in ordinary functions; preserve the
original emitter for unsupported eval/with/suspension constructs. Handle debug
scope descriptors and captures as observable uses. Re-encode all PC-dependent
metadata together after transformation. Give optimization a deterministic work
limit and a safe fallback for large/adversarial inputs.

Hermes is a useful reference for representing JavaScript semantics and effects;
adopting its entire SSA architecture is unnecessary for this first step.
[Hermes IR design][hermes-ir] Paserati already has destination hints and guarded
operand reuse; preserve those useful behaviors while making their assumptions
explicit. [Expression lowering][src-expression], [register allocator][src-regalloc]

**Acceptance:** verifier passes before/after; deterministic disassembly; optimized
and generic semantic parity; compile time, peak temporary memory, instruction
count, register high-water, and emitted bytes reported. Empty/no-op optimization
must not impose unexplained frontend overhead.

### C2. Remove provably unnecessary work

Implement in separate steps: literal primitive constant folding, redundant move
elimination/copy propagation, unreachable-block elimination, then liveness-based
register reuse. Fold only operations whose full semantics are known. Do not
reassociate floating-point arithmetic or treat addition as numeric when operands
may invoke coercion. Preserve signed zero, NaN, overflow/promotion, BigInt errors,
and evaluation order. Reuse the runtime's semantic definitions or differential
tests, rather than Go operators that silently differ.

Model exceptional successors before eliminating stores used in catch/finally.
Direct eval, mapped arguments, closures, and suspension may expose values that
ordinary local liveness misses. Keep such bindings pinned until their uses are
represented. Allocation is not generally removable just because the resulting
object is empty; identity may escape.

**Acceptance:** `return 1 + 2 * 3` reduces to loading/returning the correct constant;
side-effecting variants retain their order; long expressions stay within checked
register limits. Report compile cost and execution cost separately. Add structural
tests for intended removed work without freezing irrelevant exact register numbers.

### C3. Fuse profile-selected instruction sequences

Use opcode/pair frequency measurements from a diagnostic build to select a small
set: compare-and-branch, numeric immediates, common loads/calls, or update patterns.
Prioritize removed dispatches on representative workloads over instruction-count
reduction on synthetic cases. Track opcode-space/operand-width limits.

For each fusion, specify overlap rules, exceptional edges, side effects, source
positions, cancellation opportunities, and a semantic expansion into ordinary
instructions. Keep instrumentation out of normal timed runs. Measure code size
and mixed workloads; many handlers can worsen locality even when a microbenchmark
improves. Ignition's register-bytecode and emission optimizations and Luau's
specialized interpreter paths provide relevant precedent. [Ignition][ignition],
[Luau performance design][luau]

**Acceptance:** fused/unfused differential cases and a repeatable workload win;
no unsupported hardware-specific dependency; bounded code-size and compile-time
cost. Decline fusions whose benefits fall below measured noise or harm common
workloads beyond accepted budgets.

### C4. Add guarded specialization only after the reference mode exists

Experiment with numeric arithmetic, stable function targets, and builtin identity.
Use observed tags or proven primitive values; annotations alone cannot authorize
unchecked operations. On guard failure, execute the generic operation with the
same pre-operation state. Do not repeat a getter, conversion, or call already
performed before the failure. Record miss/fallback rates in diagnostic mode.

Prefer per-owner side metadata or same-width specialized instructions with a
verified update protocol. Repeated chunk execution and concurrent owners must
not inherit invalid assumptions. Keep cold code unspecialized and cap site state.

**Acceptance:** number-to-string/object/BigInt changes, rebound functions,
mutated builtins, proxies, reentry, and exceptions produce generic behavior.
Demonstrate amortization on realistic repetition counts and preserve cold-start
performance. This remains bytecode specialization; it requires no native JIT.

## D: property access and execution

### D1. Make the ordinary property hit cheap

After A2, put the common ordinary own-data-property IC hit near the beginning of
the get path, guarded by the actual receiver representation and shape. Avoid
walking unrelated primitive/callable cases before every ordinary hit. Keep the
generic path as the semantic authority, including receiver-sensitive accessors.
[Property get][src-getprop], [ordinary property semantics][spec-ordinary-get]

Compare prototype-chain validation designs:

| Design | Benefit | Cost and required proof |
| --- | --- | --- |
| Validate relevant chain links/shapes | Local correctness reasoning, simple first repair | Work proportional to cached chain depth |
| Owner-wide mutation epoch | Cheap check and straightforward invalidation | Unrelated mutations invalidate many sites; every relevant mutation must increment it |
| Per-chain/prototype validity cells | Stable hit cost with more selective invalidation | Dependency bookkeeping, invalidation fanout, lifetime and memory costs |

Start with the simplest correct guard. Do not adopt dependency graphs unless
mutation and depth measurements justify them. Cache negative lookups only when
the same absence/prototype contract can be maintained.

**Acceptance:** own hit, inherited hit, missing property, accessor, and mutation
workloads; warm-hit improvement and acceptable invalidation/memory cost; all A2
cases remain green. Hit percentage alone is not an acceptance metric.

### D2. Improve polymorphic and generic lookup

The [property cache][src-cache] becomes permanently megamorphic after four entries.
Prototype a bounded owner-local megamorphic cache keyed by property key and the
full layout/prototype assumptions required by the result. Use a bounded replacement
policy and measure collisions. Cache entries may themselves keep shapes/holders
alive; include them in B2's budget.

The generic metadata path scans shape fields even where other object operations
use a name index. Unify slot/descriptor lookup through a read-only indexed shape
query, preserving ordered enumeration and descriptor semantics. Consider sharing
query results between get/set/has operations only where their semantics agree.
[Property helpers][src-property-helpers]

**Acceptance:** 1/2/4/8/32-shape sites, many distinct keys, negative lookups, symbol
keys, dictionary objects, and descriptor changes. Show both stable throughput and
bounded retained state. A property cache that avoids work by returning the wrong
value is a correctness failure, regardless of benchmark speed.

### D3. Consolidate call lifetime and optimize tail argument movement

Preserve register-window ordinary calls and lazy arguments materialization.
Extract a common frame-entry/exit contract across direct, method, spread, bound,
constructor, tail, generator, async, and host reentry paths. Share lifecycle
helpers or generated code where useful, while inspecting whether abstraction
adds allocation or calls on hot paths.

The audited TCO reuses frames but allocates an argument slice per step. Prototype
overlap-safe copying into the reused window, a bounded internal scratch area,
or parallel-copy scheduling. Evaluate argument expressions before overwriting
their sources. Preserve captures, rest freshness, mapped arguments, receiver,
new.target, finally behavior, and errors before making a tail transition.

**Acceptance:** the deep-tail-recursion probe remains bounded in stack use and
avoids per-step heap growth where no observable allocation is required. Include
overlapping/cyclic argument moves and differing frame sizes, plus all B1 exit
paths. Ordinary call performance is a required canary.

### D4. Control dispatch code size and use Go tooling

The audit's arm64 `VM.run` symbol was about 278 KB; get/set handlers added about
21.6/19.8 KB. This motivates investigation, not a claim of measured instruction
cache misses. Use CPU profiles and disassembly to separate hot operations from
cold error construction and unusual object behavior. Keep hot state in a form
Go can optimize; inspect escape decisions and bounds-check elimination.

Avoid introducing a Go function call per opcode without compelling measurements.
Evaluate Go PGO with a pinned, representative training corpus and a separate
evaluation corpus. Record profile identity and build flags; compare non-PGO and
PGO lanes explicitly. PGO optimizes the Go executable at build time and does not
require a guest native JIT. [Go PGO documentation][go-pgo]

**Acceptance:** improved profiles plus workload results on supported architectures;
no broad regression hidden by one hot loop; reproducible training/build inputs;
unchanged cancellation and exception behavior.

## E: production embedding

### E1. Specify execution and resource ownership

Document the ownership/threading contract for VMs, compiled chunks, realms,
objects crossing host boundaries, and caches. The runtime already exposes
`Cancel`/`CancelVM`; audit cancellation latency during long builtin operations,
regex execution, host calls, and asynchronous draining as well as the dispatch
loop. Cleanup must respect B1. Define Reset versus disposal and which module,
global, or host state survives each operation.

Introduce per-instance logical limits for stack depth, admitted source/bundle
size, pending jobs, and guest allocations where accurately accountable. Check
large requested allocations before allocating. Explain how accounting treats
shared objects, host references, backing capacities, and caches. Enforceable
logical quotas are useful but are not exact per-VM RSS accounting. Hard process
memory limits or failure containment need process isolation; changing Go's
global GC settings inside one embedded VM is not an isolation mechanism.
[Go memory-limit scope][go-memory]

**Acceptance:** cancellation/cleanup stress tests, repeated VM creation/disposal,
independent-instance race tests, overload behavior, and sustained latency/memory
results under the documented concurrency model. No claim of hard real-time
latency or in-process tenant isolation without separate evidence.

### E2. Establish the production compatibility matrix

Keep language conformance, TypeScript checking, and host-library compatibility
as separate dimensions. Run JavaScript semantics with type checking disabled;
measure checked TypeScript compilation separately. Pin representative real
packages and offline module graphs with licenses/checksums and expected outputs.
Include reflection, diagnostics, stack traces, async completion, and host APIs.

Publish supported features, intentional exclusions, known failures, and tested
platforms from versioned data. Repair the largest relevant Test262 failure
clusters, while distinguishing optional/new-feature policy from established
core semantics. A mature production declaration needs workload and operational
evidence alongside a percentage. F4/F5 provide that evidence without burdening
every local edit.

## F: development benchmarking infrastructure

The product of this work is a development loop that answers three questions:
does the workload still execute correctly, is a change likely to matter, and
does stronger evidence confirm a regression or gain? A short check cannot
reliably detect every small slowdown. Make that limitation explicit and use
scheduled coverage to catch changes below its sensitivity.

### Existing infrastructure to extend

| Existing component | Preserve | Proposed change |
| --- | --- | --- |
| [`cmd/bench-ratchet`](../cmd/bench-ratchet/main.go) | Streaming raw capture, package/filter selection, immutable snapshots | Add bounded paired capture, explicit protocols, coverage validation, and uncertainty-aware comparison |
| [`pkg/perfdata`](../pkg/perfdata/perfdata.go) | Shared schema and raw samples | Versioned run/sample/result records with units, identities, environment, and status |
| [`ci.yml`](../.github/workflows/ci.yml) | Builds, tests, one-iteration benchmark smoke | Keep cheap correctness gates; add bounded benchmark contract tests |
| [`perf-pr.yml`](../.github/workflows/perf-pr.yml) | Opt-in `perf` label, merge-base/head on one runner, cancellation of stale jobs | Interleave sides; preserve raw results; stop treating tool failures as ordinary timing reports |
| [`perf-timeline.yml`](../.github/workflows/perf-timeline.yml) | Immutable history on `perf-data` | Comparable environment lanes, coverage gaps, accepted checkpoints, coalesced expensive work |
| [`perf-test262.yml`](../.github/workflows/perf-test262.yml) and macro scripts | Explicit heavy-run opt-in, intersection comparison | Correct A1 variant identity/policy, show lost coverage; use fixed concurrency |
| [`cmd/paserati-v8bench`](../cmd/paserati-v8bench/main.go), [`bench`](../bench/README.md) | Existing workload adapters and deterministic checksums | Pin peers and corpus; distinguish startup, compilation, and warm execution |

The current PR microcheck uses three 500 ms repetitions per benchmark on each
side, generally with the minimum reducer. It is informational. The current
ratchet compares anchor-normalized values and can retain all-time fastest values.
Neither should become a blocking timing gate unchanged. A minimum depends on
sample count and can conceal typical cost; a CPU arithmetic anchor cannot make
pointer chasing, allocation, and string operations portable across machines.

Retain the anchor as an environment diagnostic and historical display. Use raw
same-host paired times as the primary new comparison. Preserve old snapshots
under their original schema; do not reinterpret old minima as medians or mix
protocols silently.

### F1. Build a bounded capture runner

Use a versioned manifest and extend the existing command, with helpers in a
small package such as proposed `internal/benchrun`. Retain the current modes for
compatibility. The following interface is proposed and does not exist yet:

```sh
# Reuse cached binaries, measure a small fixed set and affected workloads.
go run ./cmd/bench-ratchet -base origin/main -scope auto -wall-budget 20s quick

# Fresh fixed-size confirmation, independent of screening samples.
go run ./cmd/bench-ratchet -base origin/main -scope property -wall-budget 120s confirm

# Same binary on both sides: investigate measurement noise, not code changes.
go run ./cmd/bench-ratchet -scope property -wall-budget 20s selfcheck

# Explicit broader run, suitable for a quiet worker.
go run ./cmd/bench-ratchet -base origin/main -scope full -wall-budget 10m suite
```

Resolve `-base` to an immutable revision before building; PR mode uses the PR
merge-base and records the head independently. Local dirty trees are allowed if
their complete relevant input digest is recorded and compared reproducibly;
otherwise require a clean source snapshot with an actionable diagnostic. Never
label a modified binary only with its nearest commit SHA.

Build each side once in isolated worktrees/directories, and compile Go test
binaries with `go test -c`. Run those binaries repeatedly instead of rebuilding
through `go test` per sample. Cache by source/input digest, toolchain, GOOS/GOARCH,
architecture feature settings, build tags, flags, dependency graph, PGO profile,
and workload/harness identity. Reusing a base executable is safe; reusing old base
timing as a fresh pair is not. Report build wall time separately.

Use one controller and sequential measured child processes. Default scalar
microbenchmarks to one `GOMAXPROCS` and `-test.cpu=1`; concurrent-VM workloads have
an explicit separate setting. Fix and record GC/cache settings and clear inherited
experimental flags unless requested. Never mutate global cache flags across
simultaneously executing benchmarks. Do not run compilation, profiling, or other
benchmark jobs alongside timed children on the same worker.

For each pair, execute A then B or B then A according to a balanced seeded order.
Use fresh processes to isolate global state. Run the same ordered leaf batch on
both sides and rotate batch order between pairs if the protocol specifies it.
Reset VM/workload state and warm only what the workload contract allows. Persist
each completed sample and each failure immediately, followed by a run summary.

Enforce an overall monotonic deadline in the controller and a per-child deadline.
Go's `-benchtime=50ms` targets timed work; calibration, setup, process startup,
and a single long iteration can exceed it. Estimate costs using prior captures,
stop admitting work that cannot fit, and terminate/reap hung child process trees
using platform-appropriate mechanisms. A deadline never becomes a zero-valued
sample or a pass. An interrupted run retains completed results and an explicit
incomplete status; it does not keep benchmarking after the developer cancels it.

**Acceptance:** fake child processes test success, nonzero exit, malformed output,
missing leaves, checksum failure, timeout, descendants, and cancellation. Verify
no measured children overlap. Same-binary A/A captures prove orchestration and
environment identity; they do not by themselves prove timing sensitivity.

### Tier budgets and expected use

Targets below assume warm build caches and a modest development machine. Setup
and calibration count toward measurement wall budgets; build/download time is
reported separately. Cold builds and dependency downloads must be visible and
cancellable. The caller can impose a separate overall build-plus-run deadline.

| Tier | Trigger | Work/sample plan | Initial measurement budget | Decision |
| --- | --- | --- | --- | --- |
| Smoke | Ordinary CI; local when changing benchmarks | One iteration of existing benchmark leaves; validate contracts | Aim for 30 s; bound slow leaves individually | Correctness/infrastructure gate; no speed claim |
| Quick | Developer request; selected PR checks | At most 12 small leaves, 3 fresh A/B pairs, 50 ms timed target per leaf | 20 s hard budget | Directional signal; flag suspects, missing coverage, and noise |
| Confirm | Requested for suspects or a performance PR | Usually 2–8 leaves, 12 fresh A/B pairs, 100–200 ms per leaf | 120 s hard budget | Fixed-sample regression/gain assessment |
| Suite | Scheduled main runs or explicit request | Broader fixed workload set, declared repetition counts | 10 min per shard | Catch uncovered/cumulative regressions |
| Soak/peers | Scheduled or release/performance review | Memory, latency, multi-VM, external engines | Separate 10–30 min shards | Operational evidence and broader comparisons |

For scale, 12 leaves times 50 ms times 2 sides times 3 pairs is 3.6 seconds of
nominal timed work, leaving room inside 20 seconds for calibration and startup.
It is not a promise: workloads with multi-second minimum iterations are excluded
from quick mode or get separately versioned smaller fixtures. Confirmation on
8 leaves at 200 ms with 12 pairs has 38.4 seconds of nominal timed work. If setup
makes the actual budget impossible, reduce the selected scope explicitly, run
on a worker, or return incomplete. Do not silently weaken the protocol.

Quick mode runs a fixed small canary set plus affected leaves. Keep a maximum
leaf count: matching a top-level Go benchmark can expand into dozens of subtests.
Enumerate expected leaves, estimate their costs, and validate actual expansion.
Large new fixtures do not enter the quick tier automatically.

### F2. Define workload contracts and phase separation

Store proposed manifests and small fixtures under `bench/` (for example
`bench/manifest.json` and `bench/fixtures/`). The repository ignores the top-level
`benchmarks/` directory; do not accidentally depend on an untracked corpus there.
Large external corpora need an explicit pinned fetch/cache mechanism, checksums,
licenses, and an offline error mode. Fetching is setup, never timed work.

Each workload declares a stable ID/version, implementation/input digest, adapter,
exact operation, expected result, allowed setup/warmup/reset, timing boundary,
metrics/units, tiers, estimated cost, and semantic category. A small illustrative
manifest entry for an existing Go leaf might be:

```json
{
  "id": "object.getown.last.16",
  "version": 1,
  "adapter": "go-benchmark",
  "package": "./pkg/vm",
  "selector": "^BenchmarkGetOwn$/^n=16$/^last$",
  "operation": "one ordinary own-property lookup in a 16-field object",
  "setup": "create object and keys before timing",
  "oracle": {"found": true, "integer_value": 15},
  "metrics": ["ns/op", "B/op", "allocs/op"],
  "tiers": ["smoke", "quick", "confirm", "suite"],
  "categories": ["property", "shape"],
  "runtime": {"gomaxprocs": 1, "gogc": "100", "pgo": "off"}
}
```

The current `BenchmarkGetOwn` keeps results alive but does not validate that exact
oracle. Implement contract validation when migrating it. Keep results observable
to prevent dead-code elimination, and use fixed expected values rather than merely
checking that no exception occurred. Check outside the timed loop when that does
not hide essential work; for stateful workloads include periodic or final checks
that prove all intended iterations happened. Do not redirect process-global stdout
inside concurrent benchmarks; inject a sink or isolate the process.

Separate these phases with distinct metric IDs:

| Lane | Timed boundary | Reset/warmup contract |
| --- | --- | --- |
| Parse | Source to AST | Source loaded first; new parser/AST each operation |
| Type check | Parsed program to checked result | Fresh mutable checker/AST state as required; no cached success reused |
| Compile | Prepared program to verified bytecode | Frontend/checking inclusion explicit; fresh compiler state |
| Cold CLI | Start process through validated output and exit | Startup, imports, parsing and compilation included; OS filesystem-cache state labeled |
| VM initialization | Create initialized VM/runtime | Host process already running; include builtin initialization |
| Warm execution | Invoke prepared chunk/function for fixed work | Compile/setup excluded; caches warmed by fixed workload-specific policy |
| Request/reuse | Reset/admit/execute/complete a request | Include actual reset and per-request work; persistent VM state declared |
| Allocation/retention | Allocation counters or heap after a defined lifecycle | GC policy explicit; memory probes separate from throughput |

Existing factorial/arithmetic/matrix benchmarks compile outside the timed loop
and invoke `InterpretChunk`; they do not measure frontend throughput despite some
tool comments. Preserve their true phase and add separate frontend measurements.
Do not compare different reset policies under one benchmark name.

Suggested coverage, introduced incrementally:

| Family | Quick representative | Broader/edge coverage | Work supported |
| --- | --- | --- | --- |
| Properties | Own hit, inherited hit, generic last-field lookup | 1/2/4/8/32 shapes, missing keys, accessors, mutation | A2, B2/B3, D1/D2 |
| Calls | Small ordinary call and closure capture | Spread/rest/bound/constructor/tail/reentry | A4, B1/B4, D3 |
| Numbers/control flow | Short arithmetic loop and branch loop | Mixed tags, coercion, signed zero, exceptional paths | C1–C4, B5 |
| Strings | ASCII and non-ASCII scans | Astral/lone-surrogate strings, cache collisions, large-string retention | B2/B5 |
| Arrays | Dense indexed loop and iterator loop | Holes, descriptors, inherited indices, mixed storage | A3, B5 |
| Frontend/modules | Small JS compile and small checked TS module graph | Large generated bundle, real package graph, diagnostic path | A5, C1/C2, E2 |
| Lifecycle | Small repeated request | Large temporary graphs, suspension, many instances | B1/B2/B4, E1 |

Most new representatives in this table still need implementation. Start from
existing leaves for immediate captures; do not rename them to imply coverage
they lack. Pin peer-shared JS fixtures separately from TypeScript-only features.

Select affected families through a checked-in path-to-category map. A change to
Value, bytecode, dispatch, compiler core, dependency versions, or build flags
requires broad canaries and a scheduled full run. Unknown paths trigger a
conservative selection. Always print selected and omitted families. Deletions,
renames, version changes, and unavailable peers appear as coverage changes.

Both sides must run the same workload bytes and protocol. If Go benchmark code
changes across revisions, its digest changes: use a compatible frozen harness
on both sides or mark it a new/non-comparable metric. Do not attribute a smaller
loop, weaker oracle, or altered input to engine speed. New benchmarks establish
coverage first; they cannot regress against a nonexistent baseline.

### Run records, units, and provenance

Extend `pkg/perfdata` with a new schema version while keeping legacy readers
explicit. Use a run manifest, raw sample stream, and derived report. Required
fields include:

| Record | Required information |
| --- | --- |
| Run | Schema/protocol version; run ID; timestamp; base/head SHA and dirty-input digest; harness/manifest/corpus hashes; selection and omissions; seed; deadline and completion status |
| Executable | Path-independent content hash; engine implementation/version; Go/toolchain version; tags/flags/PGO profile; dependency digest; `go version -m` where applicable |
| Environment | OS/arch/CPU model; architecture feature settings; logical CPUs; GOMAXPROCS; GC settings; cache flags; runner identity; available memory; power/thermal observations if available |
| Sample | Workload ID/version; side; pair/order; repetitions; elapsed monotonic ns; metrics with units; checksum/status; stderr artifact; timeout or failure reason |
| Report | Per-workload raw summaries and paired deltas; sample counts; dispersion; method/budget; coverage changes; regression/gain/no-signal/inconclusive/infrastructure status |

Keep allocation measurements as floating point when derived per operation.
Current integer-valued schema fields can hide fractional changes below one
allocation per operation. Preserve allocation/byte totals and operation counts
where available; a floating-point field cannot recover precision already rounded
out of Go benchmark text. For rare allocations, use a validated batched counter
measurement or an explicit fractional custom metric. Do not encode unknown or
unsupported metrics as zero.
Distinguish `bytes/op`, current live heap bytes, post-GC retained bytes, peak RSS,
and scanned bytes; their meaning and scope differ. Record process-global metrics
as such.

Use JSONL for streaming recovery and Go benchmark text for `benchstat`
interoperability. Reports must be reproducible from stored raw records without
rerunning workloads. Store local captures under ignored scratch/perf directories;
upload CI artifacts and compact immutable timeline snapshots to the existing
data branch. Retain raw evidence for accepted performance changes long enough
to investigate the corresponding release; specify retention in the workflow.

### F3. Compare without manufacturing confidence

Quick mode reports the median paired time ratio and dispersion. Three pairs
provide a useful screen, not a significant-result certificate. Initially flag
a suspect when its slowdown exceeds both a practical threshold (for example
10%) and the measured same-binary noise band for that workload/environment.
Always show the raw percentage and absolute difference even below that threshold.
Noisy or incomplete runs are marked accordingly; a quiet quick result means
only that this screen detected no change at its sensitivity.

Use an independent, fixed-size confirmation run to avoid repeatedly sampling
until a desired result appears. Freeze selected workloads, tested metrics, sample
count, and budgets before collecting its 12 pairs. If the deadline interrupts
it, return inconclusive; do not derive a blocking statistical claim from fewer
samples. A new confirmation attempt has its own identity and retained history.

A practical first blocking rule to implement and test in the reducer is:

1. For each pair and metric where smaller is better, compute
   `d = head - base - max(relativeBudget * base, absoluteFloor)`.
2. Test whether the median paired excess is positive using a one-sided exact
   paired sign test. Count strictly positive excesses as successes and retain
   exact ties as nonpositive, making the test conservative at the budget boundary.
   Missing pairs invalidate this fixed-size confirmation.
3. Correct the family of predeclared regression tests using Holm's procedure at
   family-wise 5%. Report the practical budget, paired median effect, and sample
   count beside the result. A threshold-sized difference alone is not a confirmed
   regression.
4. Treat gains as a separate declared analysis (swap sides/budgets appropriately)
   or report them descriptively. Never label a favorable three-sample screen a
   confirmed win. A tradeoff cannot be hidden inside a geometric mean.

For `n` complete pairs and `k` positive excesses, the one-sided null probability
is `sum(C(n, j), j=k..n) / 2^n`. For Holm correction, sort the `m` declared test
p-values and compare rank `i` against `0.05 / (m-i+1)`, stopping rejection at the
first failure. Keep non-comparable metrics out of the predeclared timing family
and report their status separately. The binomial calculation and correction
have standard reference implementations for checking reducer fixtures;
Paserati need not depend on Python to implement them. [Exact binomial test][binomtest],
[Holm correction reference][holm]

This conservative rule is a proposed decision policy, not a guarantee about a
noisy laptop. Correlated thermal/load drift can violate sample assumptions;
balanced order and A/A calibration are essential. Add synthetic distributions,
ties, zeros, missing pairs, outliers, and multiple-comparison fixtures to reducer
tests. Do not assume `benchstat` implements this paired policy. Use its documented
summaries/comparisons as an additional readable report, preserving the distinction
between its analysis and the chosen gate. [benchstat documentation][benchstat]

Initial budgets to calibrate: 5% for stable hot-operation throughput, 10% for
cold/request throughput, and workload-specific absolute floors for tiny
operations. Allocation-free primitive operations can have an explicit zero-
allocation contract; other allocation/byte budgets require units and a documented
workload boundary. Post-GC retention uses a separate repeated-lifecycle test,
not the timing sign test. Tail percentiles use F5's longer protocol.

Enable hard timing gates only on workload/environment lanes whose A/A history
demonstrates acceptable false-alarm behavior and usable sensitivity. Gather that
history through scheduled runs, including known injected slowdowns; record
false-positive/false-negative rates and uncertainty. Shared-runner quick checks
stay informational until this evidence exists. Benchmark correctness, missing
required coverage, and capture failure remain real failures from the beginning.

Keep raw times primary. Display anchor drift as a warning; do not divide away a
regression because the anchor happened to get slower. Prefer a same-runner
merge-base comparison for PR attribution, and a periodically rerun accepted
release checkpoint for cumulative drift. An all-time fastest result from mixed
machines must not be the gate. Accepted tradeoffs require an explicit record
with workload, reason, measurements, and the new checkpoint; historical data
must remain immutable.

### F4. Integrate with local work and CI

The fast path must be voluntary and bounded locally: no always-running background
benchmark loop and no new full-suite commit hook. Existing correctness workflow
requirements remain until deliberately changed by the project. Documentation-only
changes can skip performance measurements; benchmark definitions, tooling, corpus
locks, dependencies, and workflow changes must trigger their own contract checks.
Expand the current PR path filter, which omits several of those inputs.

Preserve the existing `perf` and `perf-test262` labels as distinct scopes. A quick
check must never launch full Test262 or peer downloads. Add an explicit confirmation
mode and publish a compact per-workload table, selected/omitted coverage, validity,
and an artifact link. Reuse the sticky-comment mechanism where authorized by the
workflow, but publish a job summary regardless of comment permissions.

Do not blanket-ignore the capture command's exit code. Separate a completed
timing report from malformed input, failed builds, crashed workloads, and missing
required results. In an informational timing job, suspected regressions may be
non-blocking; infrastructure failure is still visibly unsuccessful. Cancel older
PR runs when new commits arrive and serialize timing on each worker. Build/test
jobs may run elsewhere while benchmark measurement owns its runner.

For main history, retain lightweight snapshots and schedule/coalesce broader
comparisons rather than queueing a full suite for every push. Keep environment
lanes distinct across CPU/OS/toolchain/PGO changes and mark series breaks.
Periodically rerun a fixed release checkpoint on the same worker to catch a
series of individually small regressions. Keep release/architecture matrices
off the developer's critical path.

Test262 timing is an optional macro workload. Use the same pinned corpus,
interpretation policy, variants, timeout policy, and concurrency on both sides.
Compare the intersection of passing variants only for timing, and prominently
report newly failing, timed-out, skipped, or missing variants. A lost test cannot
improve an overall verdict. Summed per-test duration is not inherently invariant
to concurrency: contention and GC change durations. Changing the passing set
also changes a mean; a set hash reveals that break but does not correct it.

**Acceptance:** local quick runs obey deadlines; CI covers benchmark/tool changes;
failed/missing work cannot produce a valid report; reruns retain provenance;
history shows environment and coverage discontinuities; confirmation can be
requested without forcing a full suite.

### F5. Measure production behavior and peers separately

Use isolated processes for memory probes. After warmup, execute a fixed batch,
drop guest references, collect for the retention measurement, and record live
heap plus cache counts. Repeat enough batches to distinguish a plateau from
growth. `runtime.GC` belongs in this diagnostic protocol, outside throughput
timing. RSS may not immediately fall when heap objects die; report it separately.
Collect allocation volume, live heap, scan metrics, and GC CPU using supported
runtime interfaces, with metric availability checked per Go version.
[Runtime metrics][go-metrics]

Add sustained request workloads with explicit arrival model, concurrency, VM
reuse policy, input size, and GC settings. For externally driven service latency,
record queueing and end-to-end latency under an open-loop arrival schedule as
well as service time; otherwise a stalled engine can reduce offered load and
conceal tail behavior. Bound the queue and report rejected/timed-out requests.
For closed-loop embedding, label that model explicitly. Report throughput and
p50/p95/p99 only with request counts and sufficient observations; a small quick
run cannot characterize p99. Use repeated windows and record worst observed
pauses without claiming a hard latency bound.

For peer comparisons, pin Paserati, Goja, original C QuickJS, any Go-translated
QuickJS separately, and Node/V8 with `--jitless`. Record compiler/version/flags
for each. Use the same JS input bytes, verified checksums, timing boundary,
warmup, and deterministic seed. Keep original workload checks enabled. Report
unsupported workloads instead of silently taking the fastest surviving subset.
Different native builtin implementations are legitimate end-to-end differences;
do not describe a regexp-heavy result as pure bytecode-dispatch performance.

Retain the audit's V8 workloads as one historical comparison lane and add actual
package loading, JSON, strings, request allocation, closures, async suspension,
and multi-VM work. Report each workload before any declared geometric aggregate.
External engine builds, longer sampling, profiles, and soak runs happen on
scheduled/explicit jobs, not every edit.

### Benchmark infrastructure implementation order

| PR-sized step | Deliverable | Verification |
| --- | --- | --- |
| F1a | Versioned run/sample schema, required status/units, artifact reader | Round trips; legacy incompatibility; partial/error records |
| F1b | Cached executable capture, deadlines, pair/order IDs | Fake processes; no overlap; cancellation; dirty-source provenance |
| F2a | Manifest with existing leaves, exact selectors and output contracts | Expected leaf enumeration; checksum failures; changed fixture identity |
| F3a | Quick reports and A/A mode | Same-binary runs; injected changes; incomplete/noisy outcomes |
| F3b | Fixed-size confirmation and reducer tests | Synthetic statistical cases; multiple comparisons; historical replay |
| F4a | PR integration and comparable timeline lanes | Workflow dry runs; visible infrastructure failures; documentation-only skip |
| F2b/F5a | Frontend/request/memory workloads and pinned peers | Oracle validation; phase boundaries; corpus provenance |
| F4b/F5b | Scheduled release-checkpoint, soak, and calibrated gates | Noise/sensitivity history; plateau and tail-latency reports |

F1a–F3a make the short development loop useful. They do not depend on completing
the optimizer or a new Value layout. F3b and calibration should precede blocking
timing gates. Do not build a dashboard before capture validity and workload
contracts are trustworthy.

## Implementation and release gates

For each implementation PR, include the work-item ID, the behavior or measured
cost it addresses, the invariant it preserves, tests covering failure/fallback
paths, and the relevant benchmark report with revision/protocol identity. Include
compile-time and memory costs when changing representation or optimization.
Record intentionally accepted regressions explicitly, especially correctness
repairs. Keep each PR small enough to revert or bisect independently.

Use this completion checklist:

- The affected audit reproduction is fixed or its current status is documented
  with evidence; unrelated findings are not declared resolved.
- `go build ./...` and the smoke test stay green. Run relevant package tests and
  affected Test262 subsets under the corrected A1 policy when available.
- Fast-path changes have successful-hit, guard-failure, mutation, exception, and
  identity/lifetime cases as applicable. Low-impact documentation edits do not
  require invented runtime tests.
- Emitted-code changes pass the verifier and preserve source attribution;
  resource changes pass retention/cancellation tests; global-state changes get
  independent-instance race coverage.
- A valid scoped performance comparison names what it did and did not cover.
  Broader evidence accompanies changes to ubiquitous layouts or dispatch.
- Source comments and this roadmap's status are updated when an invariant or
  work-item design materially changes. Link the implementing PR and replacement
  measurements; keep historical audit observations clearly dated.

The first production-readiness milestone requires authoritative conformance
accounting, no unresolved known silent-miscompilation/identity/cache defects in
supported paths, bounded lifetime under representative sustained workloads, and
documented embedding limits. It also requires explicit treatment of remaining
core-language failures and tested application workloads. An arbitrary pass-rate
threshold or one peer benchmark win cannot substitute for those criteria.

Performance leadership is a separate claim: publish reproducible results across
declared workload families, machines, memory constraints, and interpreter modes.
Go remains the implementation language unless evidence identifies a remaining
constraint that the project cannot meet with an acceptable design. No particular
speedup multiplier is a deliverable of this roadmap.

## Appendix: reproduction recipes

### Commands available at the plan's base revision

These commands exist now; unlike F1's proposed interface, they can be used before
implementing the benchmarking plan. Run from a checkout with the required Go
toolchain. Build before testing. All generated artifacts below are ignored.

```sh
mkdir -p scratch/perf
go build ./...
go build -o scratch/perf/paserati ./cmd/paserati
go build -o scratch/perf/bench-ratchet ./cmd/bench-ratchet
go test ./tests -run TestScripts -count=1

# Check two existing property-lookup leaves; no conformance or speed verdict.
go test -run '^$' -bench '^BenchmarkGetOwn$/^n=(16|64)$/^last$' \
  -benchmem -benchtime 50ms -count 3 ./pkg/vm

# Capture a small existing set, including the anchor required by this tool.
scratch/perf/bench-ratchet -packages ./pkg/vm \
  -filter '^(BenchmarkRatchetAnchor|BenchmarkCharCodeAtScan|BenchmarkCharCodeAtScanNonASCII)$' \
  -count 3 -benchtime 50ms -out scratch/perf/current.jsonl capture
```

These are short manually scoped runs, not the proposed controller: the current
tool lacks the new overall deadline, pairing, and workload-oracle machinery.
For repeated manual experiments, compile a test binary once with
`go test -c -o scratch/perf/vm.test ./pkg/vm` and invoke its corresponding
`-test.run`, `-test.bench`, `-test.benchtime`, `-test.count`, `-test.cpu`, and
`-test.benchmem` flags. Preserve raw output and compare only identical protocols.
Go documents benchmark timing and sub-benchmark selection in the
[testing package][go-testing].

### Small source reproductions

Save each example as a separate `.js` file under `scratch/perf/`, run it with
`scratch/perf/paserati --no-typecheck path/to/example.js`, and compare with the
stated ECMAScript behavior. Expected comments below describe stdout; they are
not Paserati's `tests/scripts` last-expression expectation format. When promoting
a reproduction to that smoke suite, use a final expression or explicit runtime-
error assertion in the supported test format.

Prototype shadowing (A02):

```js
var top = {x: 1};
var middle = Object.create(top);
var leaf = Object.create(middle);
function read(o) { return o.x; }
console.log(read(leaf));
middle.x = 2;
console.log(read(leaf));
// Expected: 1, then 2. Audit observed: 1, then 1.
```

Indexed getter during iteration (A03):

```js
var a = [1];
Object.defineProperty(a, "0", {get: function () { return 42; }});
console.log(a[0]);
console.log(a.values().next().value);
for (var value of a) console.log(value);
var [first] = a;
console.log(first);
// Expected: 42 four times. Audit iteration/destructuring returned raw 1.
```

Permanent iterator exhaustion (A03):

```js
var a = [];
var it = a.values();
console.log(it.next().done);
a.push(7);
console.log(it.next().done);
// Expected: true, true. Audit observed the iterator revive.
```

Empty rest identity (A04):

```js
function f(...args) { return args; }
var a = f(), b = f();
a.push(42);
console.log(a === b);
console.log(b.length);
// Expected: false, 0. Audit observed: true, 1.
```

Switch TDZ across calls (A06):

```js
function f(n) {
  switch (n) {
    case 0: let x = 1;
    case 1: return x;
  }
}
console.log(f(0));
try { console.log(f(1)); } catch (e) { console.log(e.name); }
// Expected: 1, ReferenceError. Audit observed: 1, 1.
```

Constant-pool overflow (A05), generated without a giant committed fixture:

```sh
python3 - <<'PY'
from pathlib import Path
path = Path('scratch/perf/constant-overflow.js')
with path.open('w') as f:
    f.write('var x;\n')
    for i in range(65538):
        f.write('x = "audit-constant-' + str(i) + '";\n')
    f.write('console.log(x);\n')
PY
scratch/perf/paserati --no-typecheck scratch/perf/constant-overflow.js
```

A correct implementation prints `audit-constant-65537` or rejects the program
with a documented capacity error. The audit printed `audit-constant-1` with a
successful exit. This large-input reproduction belongs in a bounded compiler
boundary test, not a repeated quick benchmark.

### Harness and embedding probes to promote into tests

For A1, create a small isolated Test262-format fixture corpus with a minimal
harness and these deliberately invalid outcomes. Pair each with a passing
control so rejecting every test cannot satisfy acceptance:

| Metadata/fixture | Deliberate behavior | Required result |
| --- | --- | --- |
| `negative: {phase: runtime, type: TypeError}` | Complete normally | Fail: expected error absent |
| Same negative metadata | Throw `RangeError` | Fail: wrong error type |
| `negative: {phase: parse, type: SyntaxError}` | Parse successfully, throw SyntaxError at runtime | Fail: wrong phase |
| `flags: [async]` | Complete without `$DONE` | Fail/timeout under declared completion policy |
| `flags: [async]` | `$DONE(new Error("FAIL"))` | Fail: explicit async failure |
| No strictness flags | Throw only when a function's `this` is undefined | Non-strict variant passes, required strict variant fails |

The audit runner reported all six fixtures as passing. Include their actual
variant outcomes in A1's tests rather than merely checking a combined count.

For B1, construct one initialized runtime in an isolated Go process, execute a
function allocating/filling a two-million-element array and returning only its
length, drop temporary host references, force GC outside timing, and measure
retained heap before/after Reset. Repeat batches. A test-only diagnostic wipe can
establish attribution during investigation, but production acceptance must use
the real lifecycle. Test surviving captures and pending async work separately.

For B2, create/drop 50,000 single-property objects with distinct keys without
retaining them in the host, collect, and record shape/cache counts plus heap.
Repeat batches and compare with the explicitly configured cache budget. Stable-
key objects are the control. Do not globally clear caches during the measured
production protocol to manufacture a plateau.

For A7, construct independent register allocators from several goroutines under
`go test -race`. For A10, assemble a valid chunk that initializes R2, checks its
TDZ state, and returns a value; execute the same chunk twice and verify its code
boundaries remain valid. Keep the distinction between that bytecode-level defect
and the separately demonstrated switch source defect.

## References

Source links below are pinned to the plan's base so observations remain auditable
as files move. Repository-relative links elsewhere identify the implementation
areas to change. External documentation was consulted on 2026-09-05; pin corpus
and tool versions when implementing a measurement protocol.

| Reference | Why it matters |
| --- | --- |
| [Test262 interpretation rules][test262-rules] | Required variants, negative outcomes, async completion, and harness behavior |
| [ECMAScript Array iterator algorithms][spec-array-iterator] | Indexed reads, dynamic length, and completion behavior |
| [ECMAScript ordinary property get][spec-ordinary-get] | Prototype lookup and accessor receiver semantics |
| [Go GC guide][go-gc] | Allocation/retention/scan tradeoffs and collector behavior |
| [Go unsafe.Pointer rules][go-unsafe] | GC-visible references and valid pointer conversions |
| [Go runtime memory-limit API][go-memory] | Soft runtime-wide memory limit and excluded memory |
| [Go runtime metrics][go-metrics] | Supported GC, heap, allocation, and runtime observations |
| [Go testing benchmarks][go-testing] | Benchmark execution, timing, and sub-benchmark semantics |
| [Go benchstat][benchstat] | Standard raw-data reporting and statistical comparison tooling |
| [Exact binomial test][binomtest] | Reference calculation for the proposed paired decision rule |
| [Holm correction reference][holm] | Family-wise correction for multiple declared metrics |
| [Go PGO][go-pgo] | Reproducible build-time profile optimization |
| [Hermes IR design][hermes-ir] | Explicit JavaScript effects, blocks, and register allocation |
| [Ignition interpreter design][ignition] | Register bytecode and emission-time optimization precedent |
| [Luau performance design][luau] | Practical compiler/interpreter specialization tradeoffs |
| [V8 JIT-less mode][v8-jitless] | Definition of the relevant V8 interpreter comparison |
| [Original QuickJS documentation][quickjs] | Engine identity and embedding/runtime reference |

[audit-commit]: https://github.com/nooga/paserati/commit/64073a2956993079cb35209cb360397d9ba0d1f3
[plan-commit]: https://github.com/nooga/paserati/commit/2aa3681512eb69722f0322fce83cf56405afdc85
[src-test262]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/cmd/paserati-test262/main.go#L774
[src-getprop]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/op_getprop.go#L326
[src-call]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/call.go#L369
[src-vm]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/vm.go
[src-bytecode]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/bytecode.go#L790
[src-iterator]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/array_iterator.go#L103
[src-switch]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/compiler/compile_statement.go#L1862
[src-regalloc]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/compiler/regalloc.go#L75
[src-expression]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/compiler/compile_expression.go#L1289
[src-shape]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/object.go#L90
[src-ic-site]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/ic_site_cache.go#L5
[src-cache]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/cache.go#L32
[src-property-helpers]: https://github.com/nooga/paserati/blob/2aa3681512eb69722f0322fce83cf56405afdc85/pkg/vm/property_helpers.go#L1242
[test262-rules]: https://github.com/tc39/test262/blob/05bb032907160d66c212589d345fa0e335e2738c/INTERPRETING.md
[spec-array-iterator]: https://tc39.es/ecma262/multipage/indexed-collections.html#sec-createarrayiterator
[spec-ordinary-get]: https://tc39.es/ecma262/multipage/ordinary-and-exotic-objects-behaviours.html#sec-ordinaryget
[go-gc]: https://go.dev/doc/gc-guide
[go-unsafe]: https://pkg.go.dev/unsafe#Pointer
[go-memory]: https://pkg.go.dev/runtime/debug#SetMemoryLimit
[go-metrics]: https://pkg.go.dev/runtime/metrics
[go-testing]: https://pkg.go.dev/testing#hdr-Benchmarks
[benchstat]: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat
[binomtest]: https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.binomtest.html
[holm]: https://www.statsmodels.org/stable/generated/statsmodels.stats.multitest.multipletests.html
[go-pgo]: https://go.dev/doc/pgo
[hermes-ir]: https://github.com/facebook/hermes/blob/main/doc/IR.md
[ignition]: https://v8.dev/blog/ignition-interpreter
[luau]: https://luau.org/performance/
[v8-jitless]: https://v8.dev/blog/jitless
[quickjs]: https://bellard.org/quickjs/quickjs.html
