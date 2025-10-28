// no-typecheck
// expect: 0
// Test: Generator function prototype should have no own properties

var ownProperties = Object.getOwnPropertyNames(function*() {}.prototype);
ownProperties.length;
