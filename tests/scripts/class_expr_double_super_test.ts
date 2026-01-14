// expect: done
// Test: double super() call throws ReferenceError, field init runs once

let baseCtorCalled = 0;
let fieldInitCalled = 0;

class Base {
  constructor() {
    baseCtorCalled++;
  }
}

var C = class extends Base {
  field = ++fieldInitCalled;
  constructor() {
    super();
    try {
      super(); // Should throw ReferenceError
    } catch (e) {
      // Expected
    }
  }
};

new C();

// Base constructor called twice (both super() calls execute the constructor)
// But field initializer only runs once (after first successful super())
baseCtorCalled === 2 && fieldInitCalled === 1 ? "done" : "fail: base=" + baseCtorCalled + " field=" + fieldInitCalled
