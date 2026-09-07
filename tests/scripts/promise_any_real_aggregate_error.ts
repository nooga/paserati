// expect: Promise { <pending> }
// Regression test for paserati#293: Promise.any used to reject with a plain
// string message ("AggregateError: All promises were rejected") instead of a
// genuine `new AggregateError(errors, message)` instance, so `instanceof
// AggregateError` and `.errors` (used e.g. by real npm `undici`) didn't work.
//
// NOTE: this harness compares the *immediately-returned* value of the last
// statement, before the microtask queue (and so this async function) has
// run at all - like every other promise_*.ts test here, "expect" only
// proves `test()` didn't throw synchronously and returned a pending
// Promise; it does not actually check the instanceof/.errors assertion
// below. That was verified manually instead (`./paserati --no-typecheck`
// against this exact scenario, and against the CLI's own event-loop-drained
// output, both showing the real AggregateError with a 2-element .errors).
async function test() {
    try {
        await Promise.any([
            Promise.reject(new Error('a')),
            Promise.reject(new Error('b'))
        ]);
        return 'should not reach here';
    } catch (e) {
        return (e instanceof AggregateError && e.errors.length === 2) ? 'ok' : 'fail: ' + e;
    }
}

test();
