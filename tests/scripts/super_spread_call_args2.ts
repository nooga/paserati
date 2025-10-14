// Test super spread calls pass args correctly
// expect: 5,6,7,8,9
// no-typecheck

let args = [];

class Parent {
  constructor() {
    for (let i = 0; i < arguments.length; i++) {
      args.push(arguments[i]);
    }
  }
}

class Child extends Parent {
  constructor() {
    super(5, ...[6, 7, 8], 9);
  }
}

new Child();
args.join(',');
