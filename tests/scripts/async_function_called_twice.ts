// expect: r1=hello|r2=hello
// Bug fix: async functions must return correct results on repeated calls
// Previously, stale isSentinelFrame flags on reused frame slots caused
// the second call to return early with wrong results
async function getData(x) {
    const result = await ("hello");
    return result;
}

async function test() {
    const r1 = await getData(1);
    const r2 = await getData(2);
    return "r1=" + r1 + "|r2=" + r2;
}

const result = await test();
result;
