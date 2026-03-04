// expect: method,greet,true,false
function capture(value: any, context: any) {
  (capture as any).result = [context.kind, context.name, String(context.static), String(context.private)].join(",");
}

class MyClass {
  @capture
  static greet() {
    return "hello";
  }
}

(capture as any).result;
