// expect: true
// no-typecheck: the checker doesn't yet resolve an earlier closure's forward
// reference to a class declared later in the same block either (a separate,
// pre-existing gap from this test's actual target, the compiler/runtime
// binding bug) - this only exercises compiled/runtime behavior.

// #144: a closure defined textually BEFORE a function-scoped class in the
// same block, but only called AFTER the class has been declared - so there
// is no real TDZ violation, just completely ordinary, common JS - must
// still be able to capture the class. Neither #128 (closures nested inside
// the class body) nor #141 (hoisted function declarations) covers this: an
// ordinary, non-hoisted closure (a plain `const f = () => ...`) compiles at
// its own sequential position in the block's statement loop, which for this
// closure comes *before* the class's own declaration statement. Without
// pre-registering the class's storage location before ANY statement in the
// block compiles (not just hoisted ones), the closure's reference to the
// class resolved before the class had anywhere to live, baking in a
// dangling capture that can never be fixed up - the same root-cause shape
// as #128/#141, just for an ordinary sequential-position closure instead of
// a closure nested in the class body or a hoisted function.
//
// Found in a real npm package (minimatch's dist/commonjs/index.js): a
// `const minimatch = (p, pattern, options) => { ...; return new
// Minimatch(...).match(p); }` defined near the top of the module scope,
// with `class Minimatch { ... }` declared later in the same scope - real
// JS handles this trivially since minimatch() is never actually called
// until well after the whole module has finished loading.

function makeCounter(): boolean {
    // `useIt` is defined BEFORE `Counter` and compiles at this earlier
    // sequential position - it's only actually called after `Counter` has
    // been declared, below.
    const useIt = (x: number): number => new Counter(x).value;

    class Counter {
        value: number;
        constructor(x: number) {
            this.value = x;
        }
    }

    return useIt(42) === 42;
}

// A nested closure inside the earlier closure should also see the class.
function makeCounter2(): boolean {
    const makeReader = () => () => Widget.tag();

    class Widget {
        static tag(): string {
            return "widget";
        }
    }

    return makeReader()() === "widget";
}

// Mutual forward reference: the class's own method calls the earlier
// closure, and the earlier closure calls the class - both directions must
// resolve to the same, final bindings.
function makeMutual(): boolean {
    const helper = (): string => Base.tag() + "-fn";

    class Base {
        static tag(): string {
            return "base";
        }
        static combined(): string {
            return helper() + "-method";
        }
    }

    return Base.combined() === "base-fn-method";
}

// NOT fixed by this change, and deliberately not covered above: a closure
// inside a *nested* block that references a class declared in an
// *enclosing* block still throws ReferenceError - same shape as #141's own
// documented remaining gap, just for an ordinary closure instead of a
// hoisted function. See the comment above step "0.6" in compiler.go's
// BlockStatement compile case.

makeCounter() && makeCounter2() && makeMutual();
