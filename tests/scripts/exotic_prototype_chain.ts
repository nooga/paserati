// expect: true
// Regression test: a prototype chain can legally contain non-PlainObject
// values (an Array, a plain function, etc.), not just PlainObject/DictObject.
// The VM must walk past these instead of panicking or truncating the lookup.

function foo() {}
foo.prototype = new Array(1, 2, 3);
const f = new foo();
// `every` isn't own on the Array instance; it must be found by continuing
// up the chain to Array.prototype.
const arrayMethodFound = typeof f.every === "function";

function bar() {}
bar.prototype = function () {};
(bar.prototype as any).baz = 42;
const b = new bar();
// A plain function used as a prototype must still expose its own properties.
const functionProtoFound = (b as any).baz === 42;

arrayMethodFound && functionProtoFound;
