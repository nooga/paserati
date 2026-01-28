// Test that each evaluation of a RegExp literal creates a NEW object
// per ECMAScript spec, while cached compiled engines work correctly

// expect: true

// Test 1: Function returning regex literal creates new object each call
function getRegex() {
  return /foo/gi;
}

var r1 = getRegex();
var r2 = getRegex();

// Different objects (spec compliance)
var differentObjects = r1 !== r2;

// Test 2: Both work correctly (verify cached engine works)
var r1Works = r1.test('FOOBAR');
var r2Works = r2.test('foobar');

// Test 3: Direct literals in same scope create different objects
var a = /bar/;
var b = /bar/;
var directLiteralsDifferent = a !== b;

// Test 4: Verify regex actually matches
var aWorks = a.test('barcode');
var bWorks = b.test('handlebar');

// All tests must pass
differentObjects && r1Works && r2Works && directLiteralsDifferent && aWorks && bWorks;
