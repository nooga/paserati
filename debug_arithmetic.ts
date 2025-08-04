function *foo(a: any) { 
  yield a + 1; 
  return; 
}

let g = foo(3);
let result = g.next();
result.value;