// Comprehensive top-level await test
// expect: all_tests_passed

// Test 1: Simple resolved promise
const p1 = Promise.resolve(10);
const v1 = await p1;
if (v1 !== 10) {
  "test1_failed";
}

// Test 2: Await non-promise value
const v2 = await 20;
if (v2 !== 20) {
  "test2_failed";
}

// Test 3: Multiple awaits
const p3a = Promise.resolve(5);
const p3b = Promise.resolve(10);
const v3 = (await p3a) + (await p3b);
if (v3 !== 15) {
  "test3_failed";
}

// Test 4: Promise created and immediately resolved
let resolveFuncTest4: (value: number) => void;
const p4 = new Promise<number>(r => { resolveFuncTest4 = r; });
resolveFuncTest4(30);
const v4 = await p4;
if (v4 !== 30) {
  "test4_failed";
}

// Test 5: Await in expression
const v5 = (await Promise.resolve(100)) / 2;
if (v5 !== 50) {
  "test5_failed";
}

"all_tests_passed";
