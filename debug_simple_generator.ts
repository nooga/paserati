function *foo(a: any) { 
  yield a; 
}

let g = foo(42);
let result = g.next();
console.log("result:", result.value);