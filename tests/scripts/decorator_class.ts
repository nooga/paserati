// expect: decorated
function dec(value: any, context: any) {
  value.wasDecorated = true;
  return value;
}

@dec
class MyClass {}

(MyClass as any).wasDecorated ? "decorated" : "not decorated";
