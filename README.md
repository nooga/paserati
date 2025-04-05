# PASERATI

![Paserati](paserati.png)

## _"Sir, it's no V8 but we're doing what we can"_

Welcome to **PASERATI** - a _bootleg_ TypeScript runtime implementation written in Go. Unlike traditional TypeScript toolchains, Paserati runs TypeScript code directly without transpiling to JavaScript. And yes, that means it will also run _some version_ of JavaScript!

## What's Under The Hood

_Pops the hood, looks around, slams the hood shut._

Paserati aims to be performance-adjacent, like any other JavaScript engine written in Go, but with loftier ambitions to overtake all of them. It compiles TypeScript directly to bytecode for a register-based virtual machine, skipping the JavaScript middle-man entirely. Compile-time type checking will be used to specialize the bytecode for the given types which should allow for some interesting optimizations like treating some `number`s as integers, unboxed values and static method dispatch. Runtime type information is also on the roadmap.

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

For example, uh, it does tricks with functions and types.

```typescript
// It's very generic, cough, needs generics
type YCombinatorRejectedMyApplicationOnce = (
  f: (rec: (arg: any) => any) => (arg: any) => any
) => (arg: any) => any;

// The Y Combinator
const Y: YCombinatorRejectedMyApplicationOnce = (f) =>
  ((x) => f((y) => x(x)(y)))((x) => f((y) => x(x)(y)));

// Factorial function generator
const FactGen = (f: (n: number) => number) => (n: number) => {
  if (n == 0) {
    return 1;
  }
  return n * f(n - 1);
};

// Create the factorial function using the Y Combinator
const factorial = Y(FactGen);

// Calculate factorial of 5
factorial(5); // Should result in 120
```

See [tests/scripts](tests/scripts) for random bits of code.

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

Just starting up, glowplugs still on _...wheezing sounds..._. The engine turns over sometimes, but don't expect to win any races yet. Still working on getting it past the driveway without stalling on basic JS syntax.

See [docs/bucketlist.md](docs/bucketlist.md) for an up-to-date list of features.

_Slaps the roof._

## Performance

It's slow for now. Takes about 1.4 seconds to compute the factorial of 10, 1 million times, on M1 Pro.

## Contributing

Seriously, why would you want to contribute to this? _…But if you do, I’m both terrified and thrilled. PRs and issues are welcome._

## License

This project is licensed under the MIT License.

## AI Disclaimer

This project is made with heavy use of AI. Google Gemini 2.5 Pro wrote almost all the code so far under more or less careful direction and scrutiny - also known as "vibe coding but when you know what you're doing".

That fun sticker at the top of the README? It's made with GPT-4o's souped up image generation.

While I understand that some people might have reservations about using AI to write code, or using AI to do anything, I would like to reserve the right to avoid having to justify my choices.

---

_Remember: Pedal to the metal, or just pedal faster._
