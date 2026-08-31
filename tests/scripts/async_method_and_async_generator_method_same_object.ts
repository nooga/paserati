// expect: ok

// #133: an object literal with both a plain `async` method and an `async *`
// generator method, driven sequentially from another async function, used to
// panic the VM (index out of range) resuming the second one.
//
// Root cause: several manual generator-frame-resumption paths
// (resumeGenerator/resumeGeneratorWithException/resumeGeneratorWithReturn in
// pkg/vm/vm.go, plus the general call-setup path in pkg/vm/call.go used to
// start a generator/async-generator body) never cleared frame.promiseObj
// when reusing a frame slot. A stale pointer left over from an earlier,
// unrelated async function call (here, obj.foo()'s completed promise) made
// OpAwait inside the async generator's own body (gen()'s internal `await`)
// write its suspend state into that unrelated promise instead of its own -
// and a later resumption of the wrong promise ran the wrong function's
// bytecode against the wrong register window, indexing out of range.
const obj = {
    async foo() {
        return await Promise.resolve(42);
    },
    async *gen() {
        yield await Promise.resolve(1);
        yield await Promise.resolve(2);
    },
};

async function run(): Promise<string> {
    const value = await obj.foo();
    if (value !== 42) return "bad-value:" + value;

    const collected: number[] = [];
    for await (const v of obj.gen()) {
        collected.push(v);
    }
    if (collected.join(",") !== "1,2") return "bad-gen:" + collected.join(",");

    return "ok";
}

await run();
