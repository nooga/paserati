// expect: 42
function double(originalGetter: any, context: any) {
  return function(this: any) {
    return originalGetter.call(this) * 2;
  };
}

class MyClass {
  @double
  get value() {
    return 21;
  }
}

const obj = new MyClass();
obj.value;
