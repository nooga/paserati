// expect: promise created
// Basic Promise creation test

const p = new Promise((resolve, reject) => {
    resolve(42);
});

"promise created";
