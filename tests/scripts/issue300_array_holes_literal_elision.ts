// expect: true
// paserati#300: array literal elisions ([1,,3]) used to be materialized as
// a genuine `undefined` value rather than a real sparse hole - the parser
// converted every elided slot into an explicit UndefinedLiteral with no way
// for the compiler to tell it apart from a literal `undefined`. Fixed via a
// parallel ArrayLiteral.Elisions field the parser now populates and
// compileArrayLiteralElement (compile_literal.go) consults to emit the
// vm.Hole sentinel instead, so `in`/hasOwnProperty/Object.keys/etc. (already
// fixed to respect holes from `delete`/`new Array(n)` - see
// issue300_array_holes_delete_and_new_array.ts) now agree for literals too.
//
// Elisions mixed with a spread element in the same literal ([1,,...xs,,2])
// are a separate, NOT-yet-fixed gap (that path still materializes them as
// real `undefined`) - not covered here.
const checks: boolean[] = [];

const h: any = [1, , 3];
checks.push(h.length === 3);
checks.push((1 in h) === false);
checks.push(h.hasOwnProperty(1) === false);
checks.push(Object.keys(h).join(",") === "0,2");
checks.push(Object.getOwnPropertyNames(h).join(",") === "0,2,length");
checks.push(JSON.stringify(Object.entries(h)) === '[["0",1],["2",3]]');
checks.push(JSON.stringify(h) === "[1,null,3]");

const visited: number[] = [];
h.forEach((v: any, i: number) => visited.push(i));
checks.push(visited.join(",") === "0,2");

// Leading/trailing/multiple consecutive elisions.
const leading: any = [, , 1];
checks.push(leading.length === 3);
checks.push((0 in leading) === false && (1 in leading) === false);
checks.push((2 in leading) === true && leading[2] === 1);

const trailing: any = [1, ,];
checks.push(trailing.length === 2);
checks.push((1 in trailing) === false);

// A real `undefined` element (not an elision) must NOT become a hole.
const real: any = [1, undefined, 3];
checks.push((1 in real) === true);
checks.push(real.hasOwnProperty(1) === true);
checks.push(Object.keys(real).join(",") === "0,1,2");

// A large literal (forces the chunked compile path, compile_literal.go) with
// multiple elisions must still get real holes at exactly those positions.
const bigParts: string[] = [];
for (let i = 0; i < 50; i++) {
  bigParts.push(i === 20 || i === 21 ? "" : "1");
}
const big: any = eval("[" + bigParts.join(",") + "]");
checks.push(big.length === 50);
checks.push((20 in big) === false && (21 in big) === false);
checks.push((19 in big) === true && (22 in big) === true);
checks.push(Object.keys(big).length === 48);

checks.every((c) => c === true);
