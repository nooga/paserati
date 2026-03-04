// expect: method,greet,false,false
function capture(value: any, context: any) {
  const parts = [context.kind, context.name, String(context.static), String(context.private)];
  (capture as any).result = parts.join(",");
}

class MyClass {
  @capture
  greet() {
    return "hello";
  }
}

(capture as any).result;
