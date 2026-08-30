// expect: ok

// #129: same fix as async_method_object_literal_nested.ts, for the async
// *generator* shorthand-method branch (async *foo() { ... } in an object
// literal), which goes through the same unguarded body-parse.
function outer() {
    const obj = {
        async *gen() {
            yield await Promise.resolve(1);
            yield await Promise.resolve(2);
        },
    };
    return obj;
}

async function run(): Promise<string> {
    const collected: number[] = [];
    for await (const v of outer().gen()) {
        collected.push(v);
    }
    return collected.join(",") === "1,2" ? "ok" : "bad-gen:" + collected.join(",");
}

await run();
