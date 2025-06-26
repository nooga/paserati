// expect: Counter: 3
// Test static fields and methods

class StaticExample {
  // Static fields
  static count: number = 0;
  static readonly version: string = "1.0";
  static instances: StaticExample[] = [];

  // Instance field
  name: string;

  constructor(name: string) {
    this.name = name;
    StaticExample.count++;
    StaticExample.instances.push(this);
  }

  // Static methods
  static getCount(): number {
    return StaticExample.count;
  }

  static reset(): void {
    StaticExample.count = 0;
    StaticExample.instances = [];
  }

  static createDefault(): StaticExample {
    return new StaticExample("default");
  }

  // Instance method
  toString(): string {
    return `${this.name} (#${StaticExample.count})`;
  }
}

let a = new StaticExample("A");
let b = new StaticExample("B");
let c = new StaticExample("C");

`Counter: ${StaticExample.getCount()}`;
