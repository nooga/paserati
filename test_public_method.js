// no-typecheck
var callCount = 0;
var C = class {
  method([x, y, z]) {
    if (x === 1 && y === 2 && z === 3) {
      callCount = callCount + 1;
    }
  }
};

new C().method([1, 2, 3]);
console.log("callCount:", callCount);
