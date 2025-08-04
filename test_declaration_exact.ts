function *foo(a) { yield a+1; return; }

var g = foo(3);

// assert.sameValue(g.next().value, 4);
console.log("g.next().value:", g.next().value);
// assert.sameValue(g.next().done, true);
console.log("g.next().done:", g.next().done);