// Test symbol property name getters and setters
// expect: pass

let symbolGetterCalled = false;
let symbolSetterValue: number | undefined;

const sym = Symbol('testSymbol');

const obj = {
  get [sym](): number {
    symbolGetterCalled = true;
    return 42;
  },
  set [sym](value: number) {
    symbolSetterValue = value;
  }
};

// Test getter
const result = obj[sym];
if (result !== 42) {
  console.log('FAIL: symbol getter returned wrong value');
}
if (!symbolGetterCalled) {
  console.log('FAIL: symbol getter was not called');
}

// Test setter
obj[sym] = 100;
if (symbolSetterValue !== 100) {
  console.log('FAIL: symbol setter did not set value correctly');
}

'pass';
