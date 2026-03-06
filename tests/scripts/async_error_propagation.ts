// expect: caught:oops
// Bug fix: errors thrown in async functions must be catchable via try/catch and .catch()
// Previously, exception state (unwinding/crossedNative flags) leaked from
// executeAsyncFunctionBody, causing the caller's vm.run() to exit with
// InterpretRuntimeError instead of delivering the rejection to handlers.
async function willThrow() {
    throw new Error("oops");
}

async function test() {
    try {
        await willThrow();
        return "no-error";
    } catch (e) {
        return "caught:" + e.message;
    }
}

const result = await test();
result;
