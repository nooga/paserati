// expect: true
// #262: a generator body's own local const/let that shadows an *enclosing*
// function's parameter of the same name must read its own freshly-assigned
// value, not the captured outer parameter. Only affected generator bodies
// nested inside another function and returned from it - the generator's
// special OpInitYield-splicing body compilation skipped the same TDZ
// predefine pass that regular (non-generator) function bodies get from the
// general BlockStatement handling, so the local const/let never got its own
// entry in the generator's symbol table and every read of it resolved as a
// captured upvalue of the outer parameter instead.

function outerConst(n: number) {
  function* g() {
    const n = 99;
    return n;
  }
  return g;
}
const constResult = outerConst(1)().next().value === 99;

function outerLet(n: number) {
  function* g() {
    let n = 99;
    return n;
  }
  return g;
}
const letResult = outerLet(1)().next().value === 99;

// The generator's *own* parameter of the same name must still resolve to
// its own argument, not the enclosing function's parameter.
function outerOwnParam(n: number) {
  function* g(n: number) {
    return n;
  }
  return g;
}
const ownParamResult = outerOwnParam(1)(99).next().value === 99;

// A module-level (not enclosing-function-parameter) outer binding shadowed
// the same way already worked before the fix - keep it passing.
const moduleN = 1;
function* gModule() {
  const n = 99;
  return n;
}
const moduleResult = gModule().next().value === 99 && moduleN === 1;

// The bug's real-world shapes: generator methods (object literals and
// classes) route through the exact same shared compileFunctionLiteral body
// as a plain `function*` declaration, so they hit the same gap.
function outerObjMethod(n: number) {
  const obj = {
    *g() {
      const n = 99;
      return n;
    },
  };
  return obj.g;
}
const objMethodResult = outerObjMethod(1)().next().value === 99;

function outerClassMethod(n: number) {
  class C {
    *g() {
      const n = 99;
      return n;
    }
  }
  return new C().g;
}
const classMethodResult = outerClassMethod(1)().next().value === 99;

// Class async-generator method via a computed name - the exact shape #247
// hit (`async *[Symbol.asyncIterator]()`), just with the enclosing-param
// shadow from this issue instead of #247's concurrency trigger.
async function checkAsyncShapes(): Promise<boolean> {
  function outerAsyncGen(n: number): any {
    async function* g() {
      const n = 99;
      yield n;
    }
    return g;
  }
  const asyncIter = outerAsyncGen(1)();
  const asyncGenResult = (await asyncIter.next()).value === 99;

  function outerClassAsyncGen(n: number) {
    class C {
      async *[Symbol.asyncIterator]() {
        const n = 99;
        yield n;
      }
    }
    return new C();
  }
  let classAsyncGenResult = false;
  for await (const v of outerClassAsyncGen(1)) {
    classAsyncGenResult = v === 99;
    break;
  }

  return asyncGenResult && classAsyncGenResult;
}

const asyncShapesOk = await checkAsyncShapes();

constResult &&
  letResult &&
  ownParamResult &&
  moduleResult &&
  objMethodResult &&
  classMethodResult &&
  asyncShapesOk;
