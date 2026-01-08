// expect: length,name,prototype,bar
// Test that class property order follows ECMAScript spec: length, name, prototype, then user properties
class Foo { static bar() {} }
Object.getOwnPropertyNames(Foo).join(',');
