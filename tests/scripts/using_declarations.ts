// expect: b,a,throw-c,ret-d,i1,i2,loop
// Explicit resource management: `using` disposes on every exit path, in
// reverse declaration order.

const log: string[] = [];

function res(name: string) {
    return { [Symbol.dispose]() { log.push(name); } };
}

// Reverse order, and several resources in one declaration.
{
    using a = res("a"), b = res("b");
}

// Disposal still runs when the block throws.
try {
    using c = res("throw-c");
    throw new Error("boom");
} catch (e) {
}

// ...and when it returns.
function early(): number {
    using d = res("ret-d");
    return 1;
}
early();

// null and undefined resources are legal and dispose to nothing.
{
    using n = null;
    using u = undefined;
}

// for-of disposes once per iteration; the null element disposes to nothing.
for (using item of [res("i1"), res("i2"), null]) {
}

// A `using` in a plain for head lives until the loop completes.
for (using x = res("loop"); ;) {
    break;
}

log.join(",");
