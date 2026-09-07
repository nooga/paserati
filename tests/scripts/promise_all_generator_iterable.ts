// expect: Promise { <pending> }
// Regression test for paserati#293: Promise.all/allSettled/any/race all share
// vm.IterableToArray to convert their argument to an array, which used to
// panic on any iterable that wasn't a plain array/object - e.g. a generator.
async function test() {
    function* gen() {
        yield Promise.resolve(1);
        yield Promise.resolve(2);
    }
    const result = await Promise.all(gen());
    return result;
}

test();
