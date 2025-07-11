# PASERATI

![Paserati](paserati.png)

## _"Sir, it's no V8 but we're doing what we can"_

Welcome to **PASERATI** - a _bootleg_ TypeScript runtime implementation written in Go. Unlike traditional TypeScript toolchains, Paserati runs TypeScript code directly without transpiling to JavaScript. And yes, that means it will also run _some version_ of JavaScript!

## What's Under The Hood

_Pops the hood, looks around, slams the hood shut._

Paserati aims to be performance-adjacent, like any other JavaScript engine written in Go, but with loftier ambitions to overtake all of them. It compiles TypeScript directly to bytecode for a register-based virtual machine, skipping the JavaScript middle-man entirely. For now, the main optimization is inline caches (ICs) for property access - no fancy optimization wizardry, yet.

## Goals

_Lights a cigarette._

- **Quality Entertainment**: Feel the rush of running TypeScript without a bulky V8.
- **Education**: Testbed for solo large-scale software engineering with AI assistance.
- **Utility**: Eventually become a useful embedded scripting language for Go applications.
- **Decent Runtime Performance**: Beat the fastest existing JS engine implemented in Go.

## Non-goals

_Tosses 2/3 of the cigarette out the window._

- **Utility**: Don't expect this to ever replace your TypeScript toolchain. Go see [microsoft/typescript-go](https://github.com/microsoft/typescript-go) for that.
- **Performance on par with real engines**: I'm not going to make a JIT in Go, I'll stop just short of that.
- **Full feature parity with TypeScript or ECMAScript**: Keeping this in sync with all the quirks of the language is a full-time job in itself.

## Example

_Lights another cigarette, curses at the cigarette, throws it out the window._

Here's some TypeScript that actually works:

```typescript
type Cucumber = number;

const Y = <T>(f: any): any =>
  ((x: any): any => f((y: T) => x(x)(y)))((x: any): any =>
    f((y: T) => x(x)(y))
  );

const factorial = Y(
  (f: any) =>
    (n: Cucumber): Cucumber =>
      n <= 1 ? 1 : n * f(n - 1)
);

console.time("factorial");
console.log(factorial(10)); // 3628800
console.timeEnd("factorial"); // factorial: 0.016ms
```

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

The engine's warmed up and running decent up until the first bend! I've got the core language fundamentals working - variables, functions, objects, basic types, control flow. But let's be real about what's still missing:

**Big Missing Pieces:**

- **No async/await** - Don't have an event loop yet, so no async/await
- **No generators** - No `function*` or `yield`
- **No iterators** - No `Symbol.iterator` or `for...of` protocol
- **No BigInt** - Useless for now

**Also Missing:**

- WeakMap/WeakSet
- Typed Arrays, Decorators
- Namespaces

I'm not targeting any specific TypeScript or ECMAScript version - I'm just vibing in the workshop, implementing whatever seems fun or necessary. Sometimes that means skipping the hard stuff and sometimes it means going deep on random features that caught my interest.

See [docs/bucketlist.md](docs/bucketlist.md) for the complete feature rundown.

_Slaps the roof, it caves in._

## Performance and Footprint

I think it's slow, around 20x slower than V8. Takes about 626.6ms ± 29.0ms to compute the factorial of 10, 1 million times, on M1 Pro. This includes parsing, type checking, and compilation.

For comparison, [goja](https://github.com/dop251/goja) takes about 1.007s ± 0.004s on the same machine running the same benchmark with types erased.

Stripped `paserati` binary is 4.2MB, it includes the runtime, the compiler, the type checker, and the virtual machine.

## Contributing

Seriously, why would you want to contribute to this? _…But if you do, I'm both terrified and thrilled. PRs and issues are welcome._

## License

This project is licensed under the MIT License.

## AI Disclaimer

This project is made with heavy use of AI. Google Gemini 2.5 Pro and Claude Sonnet 4 wrote almost all the code so far under more or less careful direction and scrutiny - also known as "vibe coding but when you know what you're doing".

That fun sticker at the top of the README? It's made with GPT-4o's image generation.

While I understand that some people might have reservations about using AI to write code, or using AI to do anything, I would like to reserve the right to avoid having to justify my choices.

---

_Remember: Pedal to the metal, or just pedal faster._
