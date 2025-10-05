// Test multiple top-level awaits
// expect: done

const p1 = Promise.resolve(1);
const p2 = Promise.resolve(2);
const p3 = Promise.resolve(3);

const v1 = await p1;
const v2 = await p2;
const v3 = await p3;

const result = (v1 === 1 && v2 === 2 && v3 === 3) ? "done" : "fail";
result;
