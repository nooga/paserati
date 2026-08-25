// DisposableStack / AsyncDisposableStack / SuppressedError: use()/adopt()/
// defer() bookkeeping, reverse-order disposal, move() transferring resources
// to a fresh stack without disposing, generic `use<T>`/`adopt<T>` type
// inference (checked with the type checker on, unlike the Test262 run which
// disables it), and SuppressedError chaining when multiple deferred
// callbacks throw during dispose().
//
// Resources are plain objects with a computed [Symbol.dispose] method rather
// than a class defining one - a class with a computed symbol method member
// currently confuses the checker's self-assignability check, a pre-existing
// gap unrelated to DisposableStack itself.

let log: string[] = [];

function makeResource(name: string): { name: string } {
    return {
        name: name,
        [Symbol.dispose]() {
            log.push("dispose:" + name);
        },
    };
}

let stack = new DisposableStack();
let a: { name: string } = stack.use(makeResource("a"));
let bValue: string = stack.adopt("b", (v) => {
    log.push("adopt:" + v);
});
stack.defer(() => {
    log.push("defer");
});
stack.dispose();

let orderOk = log.join(",") === "defer,adopt:b,dispose:a";
let disposedOk = stack.disposed === true;
let usedTypedOk = a.name === "a";
let adoptedValueOk = bValue === "b";

// A second dispose() is a silent no-op, not a throw.
stack.dispose();

// move() transfers the resource stack without running any dispose method.
log = [];
let moveSource = new DisposableStack();
moveSource.defer(() => {
    log.push("moved-defer");
});
let moved: DisposableStack = moveSource.move();
let moveDidNotDispose = log.length === 0 && moveSource.disposed === true;
moved.dispose();
let movedDisposed = log.join(",") === "moved-defer" && moved.disposed === true;

// SuppressedError chains reverse-order dispose failures instead of losing
// all but the last one.
let suppressedOk = false;
{
    let s = new DisposableStack();
    s.defer(() => {
        throw new Error("first");
    });
    s.defer(() => {
        throw new Error("second");
    });
    try {
        s.dispose();
    } catch (e) {
        if (e instanceof SuppressedError) {
            let inner = e.suppressed;
            suppressedOk =
                e.error instanceof Error &&
                (e.error as Error).message === "first" &&
                inner instanceof Error &&
                (inner as Error).message === "second";
        }
    }
}

// `using` still works against the same [Symbol.dispose] protocol.
let usingOk = false;
{
    log = [];
    function withUsing() {
        using r = makeResource("using");
    }
    withUsing();
    usingOk = log.join(",") === "dispose:using";
}

// AsyncDisposableStack shares the same shape; disposeAsync() resolves.
let asyncStack = new AsyncDisposableStack();
let asyncDisposeRan = false;
asyncStack.defer(() => {
    asyncDisposeRan = true;
});
let asyncResult: Promise<void> = asyncStack.disposeAsync();
let asyncOk = asyncResult instanceof Promise;

orderOk &&
    disposedOk &&
    usedTypedOk &&
    adoptedValueOk &&
    moveDidNotDispose &&
    movedDisposed &&
    suppressedOk &&
    usingOk &&
    asyncOk;

// expect: true
