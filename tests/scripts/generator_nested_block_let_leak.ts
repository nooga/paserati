// expect: true
// #271: sibling gap in #262's fix (be20f91d). That commit predefined a
// generator body's own *top-level* let/const bindings before OpInitYield,
// since the generator's special statement-by-statement body compilation
// (needed to splice OpInitYield at the right place) bypasses the general
// BlockStatement "pass 0" predefine step that regular function bodies get.
// But #262's fix left `isCompilingFunctionBody` set to true across that
// whole statement-by-statement walk, so the first NESTED block statement
// it reached (an `if`'s consequence, a bare `{ }` block, ...) wrongly read
// that flag as "you are the function's own top-level body - don't create
// an enclosed scope for yourself". That predefined the nested block's own
// let/const directly into the generator's top-level symbol table (and
// never freed the register on block exit) instead of a scoped child
// table - leaking a block-scoped binding past its block and
// shadowing/clobbering any enclosing binding of the same name.

function makeIt(condition: boolean): any {
  const o: any = Outer; // outer-scope variable, closed over by the generator below
  function* g(): any {
    if (condition) {
      const o = { fake: true }; // block-scoped - must NOT leak past the if-block
    }
    return new o("real"); // must still see the OUTER `o` (the Outer class)
  }
  return g;
}
class Outer {
  tag: string;
  constructor(tag: string) {
    this.tag = tag;
  }
}
// Condition false: the if-block never runs at all - a leaked/uninitialized
// register would surface as "undefined is not a constructor".
const neverRanResult = makeIt(false)().next().value.tag === "real";
// Condition true: the if-block does run - a leaked register would surface
// as the *fake* object being used instead ("object is not a constructor",
// or silently constructing from the wrong value).
const ranResult = makeIt(true)().next().value.tag === "real";

// Same shape, but with a bare `{ }` block (no control-flow keyword at all).
function makeIt2(): any {
  const o: any = Outer2;
  function* g(): any {
    {
      const o = { fake: true };
    }
    return new o("real");
  }
  return g;
}
class Outer2 {
  tag: string;
  constructor(tag: string) {
    this.tag = tag;
  }
}
const bareBlockResult = makeIt2()().next().value.tag === "real";

// A nested `let` (not `const`) shadowing an enclosing `let` of the same name,
// read back out after the block - the non-constructor-call variant of the
// same leak.
function makeIt3(): any {
  let n = 1;
  function* g(): any {
    if (true) {
      let n = 2;
    }
    return n;
  }
  return g;
}
const letLeakResult = makeIt3()().next().value === 1;

// The bug's real-world shapes: generator methods (object literals and
// classes) and async generators route through the exact same shared
// compileFunctionLiteral body as a plain `function*` declaration, so they
// hit the same gap - one nested-block case per shape, matching the #262
// sibling test's coverage (generator_local_shadows_enclosing_param.ts).

function makeItObjMethod(): any {
  const o: any = Outer;
  const obj = {
    *g(): any {
      if (false) {
        const o = { fake: true };
      }
      return new o("real");
    },
  };
  return obj.g;
}
const objMethodResult = makeItObjMethod()().next().value.tag === "real";

function makeItClassMethod(): any {
  const o: any = Outer;
  class C {
    *g(): any {
      if (false) {
        const o = { fake: true };
      }
      return new o("real");
    }
  }
  return new C().g;
}
const classMethodResult = makeItClassMethod()().next().value.tag === "real";

async function checkAsyncShapes(): Promise<boolean> {
  function makeItAsyncGen(): any {
    const o: any = Outer;
    async function* g(): any {
      if (false) {
        const o = { fake: true };
      }
      yield new o("real");
    }
    return g;
  }
  const asyncGenResult = (await makeItAsyncGen()().next()).value.tag === "real";

  function makeItClassAsyncGen(): any {
    const o: any = Outer;
    class C {
      async *[Symbol.asyncIterator]() {
        if (false) {
          const o = { fake: true };
        }
        yield new o("real");
      }
    }
    return new C();
  }
  let classAsyncGenResult = false;
  for await (const v of makeItClassAsyncGen()) {
    classAsyncGenResult = v.tag === "real";
    break;
  }

  return asyncGenResult && classAsyncGenResult;
}
const asyncShapesOk = await checkAsyncShapes();

neverRanResult &&
  ranResult &&
  bareBlockResult &&
  letLeakResult &&
  objMethodResult &&
  classMethodResult &&
  asyncShapesOk;
