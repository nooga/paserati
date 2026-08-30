// expect: ok

// #129: an async shorthand method in an object literal must save/increment/
// restore the parser's async-function context around its own body, the same
// as every other async body does (parseFunctionLiteral, parseArrowFunction-
// BodyAndFinish, and the class-method equivalents). Without it, 'await'
// inside the method's body resolves against whatever async/non-async depth
// the *enclosing* function left behind - so defining the object literal
// inside a plain (non-async) function made 'await' mis-parse as a plain
// identifier and throw "await is not defined" instead of awaiting.
function outer() {
    const obj = {
        async foo() {
            return await Promise.resolve(42);
        },
    };
    return obj;
}

async function run(): Promise<string> {
    const value = await outer().foo();
    return value === 42 ? "ok" : "bad-value:" + value;
}

await run();
