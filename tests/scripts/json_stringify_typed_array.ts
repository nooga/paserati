// Regression test for #387: JSON.stringify(typedArray) must serialize
// indexed elements (like a plain object) and must call toJSON() when defined,
// instead of always returning "null".

let plain = JSON.stringify(new Uint8Array([1, 2, 3]));
let nested = JSON.stringify({ v: new Uint8Array([1, 2, 3]) });
let empty = JSON.stringify(new Uint8Array([]));

class WithToJSON extends Uint8Array {
  toJSON() {
    return { custom: true };
  }
}
let withToJSON = JSON.stringify(new WithToJSON([1, 2, 3]));

let correct =
  plain === '{"0":1,"1":2,"2":3}' &&
  nested === '{"v":{"0":1,"1":2,"2":3}}' &&
  empty === "{}" &&
  withToJSON === '{"custom":true}';

// expect: true
correct;
