// expect: wrapped
function log(originalMethod: any, context: any) {
  return function(this: any, ...args: any[]) {
    return "wrapped";
  };
}

class MyClass {
  @log
  greet() {
    return "hello";
  }
}

const obj = new MyClass();
obj.greet();
