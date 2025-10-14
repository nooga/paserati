// Test super() with simple object literal
// expect: 2

class Parent {
  x: number;
  constructor(obj: any) {
    this.x = Object.keys(obj).length;
  }
}

class Child extends Parent {
  constructor() {
    super({a: 1, b: 2});
  }
}

let c = new Child();
c.x;
