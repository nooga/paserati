// Test super spread calls pass correct argument count
// expect: 5
// no-typecheck

let argCount = 0;

class Parent {
  constructor() {
    argCount = arguments.length;
  }
}

class Child extends Parent {
  constructor() {
    super(5, ...[6, 7, 8], 9);
  }
}

new Child();
argCount;
