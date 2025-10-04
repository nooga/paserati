// Test static property name getters and setters (baseline)
// expect: pass

let staticGetterCalled = false;
let staticSetterValue: string | undefined;

const obj = {
  get bar(): string {
    staticGetterCalled = true;
    return 'static getter';
  },
  set bar(value: string) {
    staticSetterValue = value;
  }
};

// Test getter
const result = obj.bar;
if (result !== 'static getter') {
  console.log('FAIL: static getter returned wrong value');
}
if (!staticGetterCalled) {
  console.log('FAIL: static getter was not called');
}

// Test setter
obj.bar = 'static test';
if (staticSetterValue !== 'static test') {
  console.log('FAIL: static setter did not set value correctly');
}

'pass';
