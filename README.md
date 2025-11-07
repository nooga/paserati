# PASERATI

![Paserati](paserati.png)

## _"Sir, it's no V8 but we're doing what we can"_

Welcome to **PASERATI** - a to-be-spec-compliant ES2025 runtime with native TypeScript frontend, written entirely in Go. Unlike traditional TypeScript toolchains, Paserati type-checks and compiles TypeScript directly to bytecode without transpiling to JavaScript. And yes, that means it runs JavaScript too - by definition.

## What's Under The Hood

_Pops the hood, looks around, slams the hood shut._

Paserati compiles TypeScript/JavaScript directly to bytecode for a register-based virtual machine. The architecture includes inline caching (ICs) for property access, shape-based object optimization, and pluggable async executors with microtask scheduling. Currently prioritizing correctness over performance - the foundation for type-driven optimizations (specialization, monomorphization, unchecked fast paths) is there, just not implemented yet.

## Goals

_Lights a cigarette._

- **ECMAScript 2025 Compliance**: Currently at 81.6% Test262 language suite pass rate (19,107/23,634 tests), targeting 90%+ for production readiness.
- **Native TypeScript Execution**: Full type checking and direct bytecode compilation - no transpilation, no tsc dependency.
- **Safe Embedding**: Pluggable module resolution, execution quotas at VM level, Deno-like permission system for secure script execution in Go applications.
- **Practical Performance**: Currently prioritizing correctness, but architecture supports type-driven optimizations (specialization, monomorphization, unchecked opcodes).
- **Future Extensibility**: Once we hit 90% ES2025 compliance, building Deno/Node emulation layers on top - just for giggles.

## Non-goals

_Tosses 2/3 of the cigarette out the window._

- **Replacing TypeScript Toolchains**: Don't expect this to replace your build pipeline. Go see [microsoft/typescript-go](https://github.com/microsoft/typescript-go) for that.
- **JIT Performance**: I'm not going to make a JIT in Go, I'll stop just short of that. But we might beat the fastest pure Go JS engine.
- **Legacy JavaScript Quirks**: Targeting modern ES2025 - we don't care about `with` statements or ancient ES3 edge cases.

## Example

_Lights another cigarette, curses at the cigarette, throws it out the window._

Here's a reactive data store:

```typescript
// Helper: Type-preserving reactive wrapper
function reactive<T extends object>(target: T): T {
  return new Proxy(target, {
    set(obj, prop, value) {
      const old = obj[prop as keyof T];
      obj[prop as keyof T] = value;
      if (old !== value) console.log(`${String(prop)}: ${old} → ${value}`);
      return true;
    },
  });
}

// Generic reactive store with constraints and async methods
class ReactiveStore<T extends { id: string }> {
  private items: T[] = [];

  add(item: T): T {
    const reactiveItem = reactive(item);
    this.items.push(reactiveItem);
    return reactiveItem;
  }

  filter(predicate: (item: T) => boolean): T[] {
    return this.items.filter(predicate);
  }

  async processAll(fn: (item: T) => Promise<void>): Promise<void> {
    for (const item of this.items) await fn(item);
  }
}

interface User {
  id: string;
  name: string;
  score: number;
}

const store = new ReactiveStore<User>();
const alice = store.add({ id: "1", name: "Alice", score: 100 });
alice.score = 150; // score: 100 → 150

const topUsers = store.filter((u) => u.score > 90);
console.log(topUsers); // [{ id: "1", name: "Alice", score: 150 }]

await store.processAll(async (user) => {
  console.log(`Processed ${user.name}`);
});
```

See [examples/es2025_showcase.ts](examples/es2025_showcase.ts) for a comprehensive feature demo.

Examples may or may not work at every commit, but they should work at least once in a while.

_Wind blows the cigarette back into the car, it catches fire._

## Usage

_Turns the ignition key, there is a click, tires go flat_

```bash
# Run the REPL
./paserati

# Execute a script
./paserati path/to/script.ts

# Run the test suite
go test ./tests/...
```

## Current Status

_Scratches a nasty red spot on the roof._

**Test262 Compliance: 74.2%** (17,548/23,634 language suite tests passing)

The engine's running hot and crawling with bugs! Most ES2025 features are implemented and mostly working:

**✅ Complete:**

- **Async/Await & Promises** - Full microtask scheduling, top-level await, async generators
- **Modules** - ESM imports/exports, dynamic `import()`, pluggable module resolution
- **Classes** - Full ES2025 class syntax including private fields (`#private`), static blocks, inheritance
- **Generators** - `function*`, `yield`, `yield*` delegation
- **Advanced Types** - Generics, conditional types, mapped types, template literals, `infer` keyword
- **Modern Operators** - Optional chaining (`?.`), nullish coalescing (`??`), logical assignment
- **Destructuring** - Arrays, objects, rest/spread in all contexts
- **Built-ins** - Proxy, Reflect, Map, Set, TypedArrays, ArrayBuffer, RegExp, Symbol, BigInt
- **Eval** - Direct and indirect eval with proper scoping (bugged)

Last time I checked it could run a pure TS library [date-fns](https://github.com/date-fns/date-fns) from source without any glaring issues. _Cough, not sure if it does so at every commit, but it did at the time of writing._

**🚧 Known Gaps:**

- WeakMap/WeakSet (planned)
- Decorators (planned)
- Some ASI edge cases (eh!)
- Import attributes (experimental ES feature)

See [docs/bucketlist.md](docs/bucketlist.md) for the exhaustive yet messy feature inventory.

_Slaps the roof, it caves in._

## Performance and Footprint

Currently prioritizing correctness over speed while we push toward 90%+ Test262 compliance. Once the semantics are nailed down, the architecture is ready for type-driven optimizations - specialization to unchecked fast paths, monomorphization, and other tricks enabled by having type information at compile time.

**Footprint:** Static binary is 16MB (not stripped), includes everything - lexer, parser, type checker, compiler, VM, and full built-in library. No external dependencies.

**Architecture:** Register-based VM with inline caching for property access, shape-based object optimization (similar to V8's hidden classes), and pluggable async executor for embedding flexibility.

## Contributing

Seriously, why would you want to contribute to this? _…But if you do, I'm both terrified and thrilled. PRs and issues are welcome._

## License

This project is licensed under the MIT License.

## AI Disclaimer

This is a **one-man** project written in my **free time** with the help of **AI**. It is also an experiment in large scale software engineering with AI aimed at delivering a production-quality open source project.

Google Gemini 2.5 Pro and Claude Sonnet 4/4.5 wrote almost all the code so far under more or less careful direction and scrutiny - also known as "vibe coding but when you know what you're doing".

That fun sticker at the top of the README? It's made with GPT-4o's image generation.

---

_Remember: Pedal to the metal, or just pedal faster._
