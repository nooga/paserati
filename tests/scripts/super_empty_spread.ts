// Test super() with trailing empty spread
// expect: 3
// no-typecheck

let argCount = 0;

class Parent {
  constructor() {
    argCount = arguments.length;
  }
}

class Child extends Parent {
  constructor() {
    super(1, 2, 3, ...[]);
  }
}

new Child();
argCount;
