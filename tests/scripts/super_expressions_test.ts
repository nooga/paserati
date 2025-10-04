// expect: undefined
class Test262ParentClass {
  constructor(a: number, b: number, c: number, d: number, e: number) {
    // Should receive 1, 2, 3, 4, 5
    console.log("Parent constructor called with:", a, b, c, d, e);
    console.log("Arguments length:", arguments.length);
  }
}

class Test262ChildClass extends Test262ParentClass {
  constructor() {
    super(1, 2, ...[3, 4, 5]);
  }
}

const instance = new Test262ChildClass();
console.log("Instance created successfully");
