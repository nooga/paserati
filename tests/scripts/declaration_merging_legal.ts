// expect: 3
// Legal merges are not duplicate-declaration errors: interface + class,
// overloaded functions, a var redeclared with the same type, and enum + enum.

interface Box {
  extra: number;
}
class Box {
  value = 1;
}

function pick(x: number): number;
function pick(x: string): string;
function pick(x: any): any {
  return x;
}

var count: number = 1;
var count: number = 2;

const b = new Box();
b.extra = 2;
b.value + b.extra;
