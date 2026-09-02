// expect: 10
// #203/#204: object-literal methods' PropertyName is an IdentifierName, which
// includes every reserved word - async/generator methods and plain methods
// alike must accept reserved-word names like real Node does.
const o = {
  import(e: number) { return e; },
  async class(e: number) { return e; },
  static(e: number) { return e; },
  implements(e: number) { return e; },
  *interface(e: number) { yield e; },
};

o.class(5).then((v: number) => console.log("async class:", v));

let total = o.import(1) + o.static(2) + o.implements(3);
for (const v of o.interface(4)) total += v;

total;
