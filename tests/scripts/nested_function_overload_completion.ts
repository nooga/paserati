// expect: 2
function outer() {
  function inner(x: number): number;
  function inner(x: string): string;
  function inner(x: any) {
    return x;
  }

  return inner(2);
}

outer();
