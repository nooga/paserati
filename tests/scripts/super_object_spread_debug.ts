// Test super() with object spread - debug version
// expect: object
// no-typecheck

let o = {};
Object.defineProperty(o, "b", {value: 3, enumerable: false});

let argType = "";

class Parent {
  constructor(obj: any) {
    argType = typeof obj;
  }
}

class Child extends Parent {
  constructor() {
    super({...o});
  }
}

new Child();
argType;
