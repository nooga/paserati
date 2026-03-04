// expect: d2,d1
const order: string[] = [];

function d1(value: any, context: any) {
  order.push("d1");
}

function d2(value: any, context: any) {
  order.push("d2");
}

@d1
@d2
class MyClass {}

order.join(",");
