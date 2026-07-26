// expect: 42
// TS2454 must stay quiet whenever a write is guaranteed to precede the read.

function assignedFirst(): number {
    let a: number;
    a = 1;
    return a;
}

// Both arms of the conditional assign, so the merge is definitely assigned.
function bothBranches(flag: boolean): number {
    let b: number;
    if (flag) {
        b = 2;
    } else {
        b = 3;
    }
    return b;
}

// The `then` arm cannot fall through, so the `else` arm decides on its own.
function terminatingBranch(flag: boolean): number {
    let c: number;
    if (flag) {
        return 10;
    } else {
        c = 4;
    }
    return c;
}

// A `!` assertion opts the declaration out entirely.
function definiteAssertion(): number {
    let d!: number;
    d = 5;
    return d;
}

// A type admitting undefined needs no initializer.
function admitsUndefined(): number {
    let e: number | undefined;
    return e === undefined ? 6 : e;
}

// do/while runs its body at least once, so the write propagates out.
function doWhileRuns(): number {
    let g: number;
    do {
        g = 7;
    } while (false);
    return g;
}

// Writes from a loop body are assumed to have happened for later reads.
function afterLoop(items: number[]): number {
    let h: number;
    for (const item of items) {
        h = item;
    }
    return h;
}

// Assignment through a nested function is untrackable, so no diagnostic.
function viaClosure(): number {
    let i: number;
    const set = () => { i = 9; };
    set();
    return i;
}

assignedFirst() + bothBranches(true) + terminatingBranch(false) +
    definiteAssertion() + admitsUndefined() + doWhileRuns() +
    afterLoop([8]) + viaClosure();
