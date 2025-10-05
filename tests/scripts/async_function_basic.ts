// expect_runtime_error: OpAwait
// Basic async function parsing test
// This should type check successfully but will fail at runtime until we implement OpAwait

async function test() {
    return 42;
}

const result = test();
console.log(result);
