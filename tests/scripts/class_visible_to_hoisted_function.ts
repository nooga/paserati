// expect: true
// no-typecheck: the checker doesn't yet resolve a hoisted function's forward
// reference to a class declared later in the same block either (a separate,
// pre-existing gap from this test's actual target, the compiler/runtime
// binding bug) - this only exercises compiled/runtime behavior.

// #141: a class declared inside a function body must also be visible to a
// *hoisted function declaration* sibling in the same scope, not just to
// arrow functions/methods/inline closures nested inside the class body
// itself (which #128 already fixed).
//
// Function declarations are hoisted and their bodies compiled at the top of
// their enclosing block, before any other statement in that block -
// including a class declaration that comes later in source order. Without
// pre-registering the class's storage location before that hoisting step,
// the hoisted function's reference to the class resolved before the class
// had allocated anywhere to live, baking in a dangling capture (same
// root-cause shape as #128, but for a hoisted-function sibling instead of a
// closure nested directly inside the class body).
function makeCounter(): boolean {
    // `helper` is hoisted above `Counter` and its body compiles first.
    function helper(): number {
        return Counter.count;
    }

    class Counter {
        static count: number = 42;
    }

    return helper() === 42;
}

// A nested closure inside the hoisted function should also see the class.
function makeCounter2(): boolean {
    function makeReader(): () => string {
        return () => Widget.tag();
    }

    class Widget {
        static tag(): string {
            return "widget";
        }
    }

    return makeReader()() === "widget";
}

// Mutual forward reference: the class's own method calls the hoisted
// function, and the hoisted function calls the class - both directions must
// resolve to the same, final bindings.
function makeMutual(): boolean {
    function helper3(): string {
        return Base.tag() + "-fn";
    }

    class Base {
        static tag(): string {
            return "base";
        }
        static combined(): string {
            return helper3() + "-method";
        }
    }

    return Base.combined() === "base-fn-method";
}

// NOT fixed by this change, and deliberately not covered above: a function
// hoisted inside a *nested* block that references a class declared in an
// *enclosing* block still throws ReferenceError -
//
//   function outer() {
//       {
//           function helper() { return C.tag(); }
//           helper(); // ReferenceError: C is not defined
//       }
//       class C {
//           static tag() { return "x"; }
//       }
//   }
//
// This fix only pre-registers a class against sibling hoisted functions in
// the *same* block (collectClassDeclarations only looks at that block's own
// statement list) - same root-cause shape, but crossing into an enclosing
// scope is a separate, deeper problem. See the comment above step "0.6" in
// compiler.go's BlockStatement compile case.

makeCounter() && makeCounter2() && makeMutual();
