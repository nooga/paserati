// Test __proto__ in object literals
// expect: pass

const parent = { x: 42, y: 100 };
const child = {
  __proto__: parent,
  z: 10
};

// Test prototype chain
if (child.z !== 10) {
  console.log('FAIL: own property not accessible');
}
if (child.x !== 42) {
  console.log('FAIL: inherited property not accessible');
}
if (child.y !== 100) {
  console.log('FAIL: second inherited property not accessible');
}

// Test Object.getPrototypeOf
if (Object.getPrototypeOf(child) !== parent) {
  console.log('FAIL: prototype not set correctly');
}

// Test that __proto__ is not an own property
if (child.hasOwnProperty('__proto__')) {
  console.log('FAIL: __proto__ should not be an own property');
}

// Test __proto__: null
const nullProto = { __proto__: null, a: 1 };
if (Object.getPrototypeOf(nullProto) !== null) {
  console.log('FAIL: __proto__: null did not set null prototype');
}

// Test non-object __proto__ (should be ignored per spec)
const ignored = { __proto__: 42, b: 2 };
if (Object.getPrototypeOf(ignored) !== Object.prototype) {
  console.log('FAIL: non-object __proto__ should be ignored');
}

'pass';
