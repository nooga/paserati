function *foo(a: number) { 
  console.log("a type:", typeof a);
  console.log("a value:", a);
  yield a + 1; 
  return; 
}

let g = foo(3);
g.next();