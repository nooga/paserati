// Test super() with object spread from outer scope
// expect: 3

let base = {a: 1, b: 2};

class Parent {
  x: number;
  constructor(obj: any) {
    this.x = Object.keys(obj).length;
  }
}

class Child extends Parent {
  constructor() {
    super({...base, c: 3});
  }
}

let c = new Child();
c.x;
