// expect: prefixed:hello
function prefix(p: string) {
  return function(originalMethod: any, context: any) {
    return function(this: any, ...args: any[]) {
      return p + ":" + originalMethod.call(this, ...args);
    };
  };
}

class MyClass {
  @prefix("prefixed")
  greet() {
    return "hello";
  }
}

const obj = new MyClass();
obj.greet();
