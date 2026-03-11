// Test that methods inherited from a decorated base class are accessible via this
// This covers the anjin Agent pattern: extending a generic abstract class
// and accessing inherited methods like tools()

function log(method: any, context: any): any {
  return method;
}

class Base {
  items(): string[] {
    return ["a", "b"];
  }
}

class Child extends Base {
  @log
  greet(): string {
    return "hello";
  }

  doStuff(): string {
    const items = this.items();
    return items.length + ":" + this.greet();
  }
}

const c = new Child();
c.doStuff();

// expect: 2:hello
