// expect: class,MyClass
function capture(value: any, context: any) {
  (capture as any).result = context.kind + "," + context.name;
}

@capture
class MyClass {}

(capture as any).result;
