// expect: no-loop:REPLACED|loop:REPLACED|finally:FINALLY_REPLACED|string:STRING_REPLACED
// Regression test for #422: when an async function's own catch/finally
// block, entered by an awaited rejection, itself throws a DIFFERENT value
// than the one it caught, that NEW value must be what the async function's
// promise rejects with - not the original awaited rejection reason.
//
// Root cause: the internal await-rejection resumption path
// (resumeAsyncFunctionWithException's Reject reaction in pkg/vm/vm.go)
// rejected the async function's promise with the closure's captured
// `reason` (the ORIGINAL awaited rejection) instead of the exception
// actually returned by resumeAsyncFunctionWithException, which may have
// been replaced along the way. This was masked whenever a rethrow reused
// the same value it caught - only a genuine substitution exposed it.
//
// The bug's own report guessed a loop was a necessary ingredient (a plain,
// non-looped try/catch "did not reproduce"); it isn't - the loop was
// incidental, and every shape below (no loop, loop, finally, non-Error
// thrown value) hit the same bug before the fix. Covering all four here
// so the invariant, not one instance of it, stays pinned.
async function refresh() {
    throw new Error("INNER");
}

async function noLoop() {
    try {
        await refresh();
    } catch (e) {
        throw new Error("REPLACED");
    }
}

async function withLoop() {
    for (let i = 0; i < 1; i++) {
        let lastError = new Error("REPLACED");
        try {
            await refresh();
        } catch (refreshError) {
            throw lastError;
        }
    }
}

async function withFinally() {
    try {
        await refresh();
    } finally {
        throw "FINALLY_REPLACED";
    }
}

async function withStringThrow() {
    try {
        await refresh();
    } catch (e) {
        throw "STRING_REPLACED";
    }
}

async function run(label: string, fn: () => Promise<void>): Promise<string> {
    try {
        await fn();
        return label + ":no-error";
    } catch (e) {
        return label + ":" + (e instanceof Error ? e.message : e);
    }
}

const results = [
    await run("no-loop", noLoop),
    await run("loop", withLoop),
    await run("finally", withFinally),
    await run("string", withStringThrow),
];
results.join("|");
