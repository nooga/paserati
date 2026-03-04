// expect: wrapped-static
function wrap(originalMethod: any, context: any) {
  return function(this: any, ...args: any[]) {
    return "wrapped-static";
  };
}

class MyClass {
  @wrap
  static greet() {
    return "hello";
  }
}

MyClass.greet();
