// expect: 1
// An async callback against a union parameter that holds the callable
// member: Out is inferred from the awaited return, and the body is not
// checked against the still uninferred Out.
function use<Out>(fn: ((o: {}) => Promise<Out>) | number): Out {
  return 1 as Out;
}
const r: number = use(async (o) => 1);
r;
