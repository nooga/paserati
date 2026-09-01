// expect: 1,2|3|4|5|6,7|8|9|10|11|12,13|14,15|56,57,[58,59]|60,{"h":61,"i":62}|16,[17,18]|19,{"m":20}|21|22|23,23|25,25|27|57|57
// A namespace member exported via a destructuring pattern never landed on the
// namespace object: `namespace N { export const {a} = obj; }` left N.a
// undefined at runtime, and absent from N's type (TS2339).
//
// This was the third of three export-registration paths with the same gap. The
// module-level ones just needed the bindings enumerated, because those
// bindings already exist as globals. The namespace path instead *rewrites* the
// declaration - compileNamespaceVarLikeExports turns `export const x = v` into
// `<ns>.x = v` - and a pattern has no single Name/Value to rewrite that way, so
// it had no case at all.
//
// The fix rewrites an exported destructuring declaration into the equivalent
// destructuring *assignment* onto the namespace object:
//
//   export const {a, b: c} = obj    ->    ({a: N.a, b: N.c} = obj)
//
// which keeps the invariant that an exported namespace binding *is* the
// namespace property, so internal reads/writes and external observation can't
// drift apart (asserted below).

let out: string[] = [];

// --- the repro, and each keyword ---
namespace A {
  export const { a } = { a: 1 };
  export const b = 2;
}
out.push([A.a, A.b].join(","));

namespace B {
  export const [a] = [3];
}
out.push(String(B.a));

namespace C {
  export let { a } = { a: 4 };
}
out.push(String(C.a));

namespace D {
  export var { a } = { a: 5 };
}
out.push(String(D.a));

// --- a pattern mixed into a declarator list (paserati#159's shape). The clause
// desugars to a DeclarationGroup, which parseBlockStatement splices into the
// namespace body as two separate ExportNamedDeclarations - so the namespace
// code never sees a group, and needs no branch for one.
namespace E {
  export const a = 6,
    { b } = { b: 7 };
}
out.push([E.a, E.b].join(","));

namespace F {
  export const { b } = { b: 8 },
    a = 9;
}
out.push(String(F.b));
out.push(String(F.a));

// --- renamed target ---
namespace G {
  export const { x: a } = { x: 10 };
}
out.push(String(G.a));

// --- nested patterns. The shorthand form is the one that discriminates: the
// inner ObjectProperty has Key=g and Value=nil, so retargeting Key in place
// would lose the source property name - it has to expand to `g: <ns>.g`.
namespace H {
  export const {
    f: { g },
  } = { f: { g: 11 } };
}
out.push(String(H.g));

namespace I {
  export const {
    f: { g: h, k },
  } = { f: { g: 12, k: 13 } };
}
out.push([I.h, I.k].join(","));

namespace J {
  export const { f: [h] } = { f: [14] };
  export const [[i]] = [[15]];
}
out.push([J.h, J.i].join(","));

// A nested default and a nested rest. These were verified only with
// --no-typecheck when this test was written, because the checker rejected them
// ("invalid destructuring target type: *parser.AssignmentExpression /
// *parser.SpreadElement") independently of namespaces - so retargetPattern's
// two wrapper branches had no typed coverage. They do now.
namespace JD {
  export const {
    f: { g = 56 },
  } = { f: {} };
  export const [[x, ...ys]] = [[57, 58, 59]];
}
out.push([JD.g, JD.x, JSON.stringify(JD.ys)].join(","));

// A rest element nested inside an object pattern. retargetPattern reaches it
// through ObjectProperty.Key rather than as a target, so the shorthand branch
// used to treat the SpreadElement itself as the property name and bind nothing -
// N.rr came back undefined while its sibling was correct.
namespace JR {
  export const {
    f: { g: jrg, ...jrRest },
  } = { f: { g: 60, h: 61, i: 62 } };
}
out.push(jrJoin());
function jrJoin(): string {
  return JR.jrg + "," + JSON.stringify(JR.jrRest);
}

// --- defaults, rest and elision ---
namespace K {
  export const [a, ...r] = [16, 17, 18];
}
out.push([K.a, JSON.stringify(K.r)].join(","));

namespace L {
  export const { a, ...r } = { a: 19, m: 20 };
}
out.push([L.a, JSON.stringify(L.r)].join(","));

namespace M {
  export const { a = 21 } = {};
}
out.push(String(M.a));

namespace N {
  export const [, a] = [99, 22];
}
out.push(String(N.a));

// --- the exported binding IS the namespace property, both directions.
// A write from inside the namespace is visible on the namespace object...
namespace O {
  export let { a } = { a: 22 };
  export function bump(): number {
    a = a + 1;
    return a;
  }
}
out.push([O.bump(), O.a].join(","));

// ...and a write to the namespace object is visible inside.
namespace P {
  export let { a } = { a: 0 };
  export function read(): number {
    return a;
  }
}
P.a = 25;
out.push([P.a, P.read()].join(","));

// --- nested namespaces, and the type is the real one (not any): assigning
// Q.I.a to a number is only legal because the checker recorded its type.
namespace Q {
  export namespace I {
    export const { a } = { a: 27 };
  }
}
const typed: number = Q.I.a;
out.push(String(typed));

// --- a namespace whose object is a local rather than a global slot: the
// retargeted MemberExpression is compiled at a different point than the
// original declaration, so it has to resolve the same accessExpr either way.
function scoped(): number {
  namespace R {
    export const { a } = { a: 28 };
    export const [b] = [29];
  }
  return R.a + R.b;
}
out.push(String(scoped()));

{
  namespace S {
    export const { a } = { a: 57 };
  }
  out.push(String(S.a));
}

out.join("|");
