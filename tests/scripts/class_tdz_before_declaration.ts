// expect: true
// no-typecheck: same reason as class_visible_to_earlier_closure.ts - the
// checker rejects these forward references independently; this exercises
// compiled/runtime behavior only.

// #143 review (@mparrett): the block-level class pre-registration added for
// #141/#144 reserved a spill slot for every class declared in a block, but
// never seeded it. An unwritten spill slot reads as `undefined`, and NEITHER
// spilled-identifier load path checked for the TDZ sentinel, so
// pre-registration silently DELETED TDZ for class bindings: an access before
// the class's own declaration stopped throwing ReferenceError and quietly
// produced the raw sentinel value instead.
//
// The fix has two halves, both required:
//   1. compiler.go step 0.6 marks the binding TDZ (DefineTDZSpilled) and
//      emits OpLoadUninitialized + OpStoreSpill to seed the sentinel before
//      any statement in the block runs.
//   2. Both `symbolRef.IsSpilled` identifier branches emit
//      OpCheckUninitialized when the symbol is still TDZ at compile time.
//
// No check is emitted on the upvalue path: OpLoadFree already does an
// unconditional runtime TDZ check inside the VM, which is better than an
// emitted OpCheckUninitialized would be because it is not subject to that
// opcode's self-rewrite-to-OpNop. Verified by removing an equivalent emitted
// check and confirming every case below is unchanged.

// --- Matt's two reported cases -------------------------------------------

// (a) direct access before the declaration
function directEarlyAccess(): boolean {
    let caught = false;
    try {
        C;
    } catch (e) {
        caught = e instanceof ReferenceError;
    }
    class C {}
    return caught;
}

// (b) an earlier closure CALLED before the declaration. The capture is legal
// (that is #144); calling it this early is not.
function earlyClosureCall(): boolean {
    const useIt = () => new D(1).v;
    let caught = false;
    try {
        useIt();
    } catch (e) {
        caught = e instanceof ReferenceError;
    }
    class D {
        v: number;
        constructor(x: number) {
            this.v = x;
        }
    }
    return caught;
}

// (c) the #144 case must still work: same earlier closure, called AFTER the
// declaration. Guards against "fixing" TDZ by reverting #144.
function laterClosureCall(): boolean {
    const useIt = () => new E(1).v;
    class E {
        v: number;
        constructor(x: number) {
            this.v = x;
        }
    }
    return useIt() === 1;
}

// --- cases the TDZ machinery could plausibly get wrong --------------------

// (d) OpCheckUninitialized self-rewrites to OpNop on a SUCCESSFUL check, so a
// one-shot check would stop throwing after the first pass. Calling the same
// function twice must throw both times.
function twiceCalled(): boolean {
    let caught = false;
    try {
        F;
    } catch (e) {
        caught = e instanceof ReferenceError;
    }
    class F {}
    return caught;
}

// (e) The dangerous ordering for that same rewrite: run the HAPPY path at a
// code site first, then hit the early path at that same site. Measured to
// still throw, because the closure's guard is the VM's unconditional
// OpLoadFree check rather than the self-rewriting opcode.
function happyPathFirst(early: boolean): string {
    const useIt = () => new G(1).v;
    if (early) {
        try {
            return "no-throw";
        } catch (e) {
            return "unreachable";
        }
    }
    class G {
        v: number;
        constructor(x: number) {
            this.v = x;
        }
    }
    return "ok:" + useIt();
}

function happyThenEarly(): boolean {
    const useIt = () => new H(1).v;
    let caught = false;
    try {
        useIt();
    } catch (e) {
        caught = e instanceof ReferenceError;
    }
    class H {
        v: number;
        constructor(x: number) {
            this.v = x;
        }
    }
    // after the declaration the very same closure must work
    return caught && useIt() === 1;
}

// (f) A class in a LOOP body: the sentinel must be re-seeded per iteration,
// not left holding the previous iteration's constructor (#132 gave loop
// bodies per-iteration bindings).
function loopBody(): boolean {
    const results: boolean[] = [];
    for (let i = 0; i < 2; i++) {
        let caught = false;
        try {
            I;
        } catch (e) {
            caught = e instanceof ReferenceError;
        }
        class I {}
        results.push(caught);
    }
    return results.length === 2 && results[0] && results[1];
}

directEarlyAccess() &&
    earlyClosureCall() &&
    laterClosureCall() &&
    twiceCalled() &&
    twiceCalled() &&
    happyPathFirst(false) === "ok:1" &&
    happyThenEarly() &&
    loopBody();
