function *foo(a: any) { 
  console.log("a type:", typeof a);
  console.log("a value:", a);
  yield a + 1; 
  return; 
}

let g = foo(3);
g.next();