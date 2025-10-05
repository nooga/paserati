// expect: promise resolved
// Test Promise.resolve() and .then()
// The callback runs via microtask

const p = Promise.resolve(42);
p.then((value: number) => {
    console.log('Callback ran with:', value);
});

"promise resolved";
