// Test loose equality (==) type coercion fixes
// expect: pass

// Test hex string to number using === to avoid type checker warnings
// Runtime will handle the coercion via string-to-number conversion
function testHex() {
  const result = (255 as any) == ("0xff" as any);
  if (!result) throw new Error('255 should equal "0xff"');
}
testHex();

// Test Boolean wrapper object
const boolWrapper: any = new Boolean(true);
if (boolWrapper != 1) {
  throw new Error('new Boolean(true) should equal 1');
}

// Test object with valueOf() method
const objWithValueOf: any = {
  valueOf() {
    return 42;
  }
};
if (objWithValueOf != 42) {
  throw new Error('Object with valueOf() should equal its primitive value');
}

// Verify strict equality still works correctly
if (new Boolean(true) === 1) {
  throw new Error('new Boolean(true) should NOT strictly equal 1');
}

'pass';
