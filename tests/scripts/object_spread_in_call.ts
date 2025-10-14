// Test object spread in regular function call
// expect: true
// no-typecheck

let o = {};
Object.defineProperty(o, "b", {value: 3, enumerable: false});

function test(obj: any) {
  return !obj.hasOwnProperty("b") && Object.keys(obj).length === 0;
}

test({...o});
