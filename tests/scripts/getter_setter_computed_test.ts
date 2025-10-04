// Test computed property name getters and setters
// expect: pass

let getterCalled = false;
let setterValue: string | undefined;

const propName = 'foo';

const obj = {
  get [propName](): string {
    getterCalled = true;
    return 'getter result';
  },
  set [propName](value: string) {
    setterValue = value;
  }
};

// Test getter
const result = obj.foo;
if (result !== 'getter result') {
  console.log('FAIL: getter returned wrong value');
}
if (!getterCalled) {
  console.log('FAIL: getter was not called');
}

// Test setter
obj.foo = 'test value';
if (setterValue !== 'test value') {
  console.log('FAIL: setter did not set value correctly');
}

'pass';
