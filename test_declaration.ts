function *foo(a: any) { yield a+1; return; }

var g = foo(3);

console.log(g.next().value);
console.log(g.next().done);