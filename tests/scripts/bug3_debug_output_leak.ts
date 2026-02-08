// expect: 42
function id<T>(x: T): T { return x; }
id(42);
