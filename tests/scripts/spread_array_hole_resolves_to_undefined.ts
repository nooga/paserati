// expect: true
// Array spread ([...arr], a function call's ...arr, etc.) iterates its
// source through the array's default iterator (%Array.prototype%[Symbol.
// iterator]), which reads each index via ordinary [[Get]] - and [[Get]] on
// a hole (from `delete arr[i]`, `new Array(n)`, or `arr.length = n` growing
// past the backing storage) always yields a genuine, PRESENT `undefined`,
// never "no property at all". So a hole in the SOURCE must never survive
// into the spread RESULT as a hole - the result must have a real, present
// `undefined` at that index instead.
//
// extractSpreadArguments (pkg/vm/vm.go) used to fast-path TypeArray sources
// with a raw `copy(args, arrayObj.elements)`, which carried a Hole value
// straight through unchanged, AND silently truncated the result to
// len(elements) instead of the array's actual .length whenever the two
// diverge (SetLength deliberately never grows the elements slice on its
// own - see `new Array(n)` and `arr.length = n` below). Both are fixed by
// reading every index 0..length-1 through ArrayObject.Get, which already
// resolves a Hole (and any index past the elements slice, up to length) to
// Undefined.
//
// Note: on this branch, a literal elision (`[1,,3]`) is not yet a real
// hole - it's a separate, already-fixed gap tracked elsewhere (paserati
// #300) that isn't merged here. It's still included below since it must
// keep behaving correctly (present undefined) either way, before or after
// that separate fix lands.
const checks: boolean[] = [];

function present(arr: any, i: number): boolean {
  return arr.hasOwnProperty(i) && i in arr;
}

// A hole from `delete`.
const deleted: any[] = [1, 2, 3];
delete deleted[1];
const spreadDeleted: any = [...deleted];
checks.push(spreadDeleted.length === 3);
checks.push(present(spreadDeleted, 1));
checks.push(spreadDeleted[1] === undefined);
checks.push(JSON.stringify(spreadDeleted) === "[1,null,3]");
checks.push(Object.keys(spreadDeleted).join(",") === "0,1,2");

// A hole from `new Array(n)` - the elements slice starts out completely
// empty; every index up to length is a hole.
const sparse: any = new Array(3);
const spreadSparse: any = [...sparse];
checks.push(spreadSparse.length === 3);
checks.push(present(spreadSparse, 0) && present(spreadSparse, 1) && present(spreadSparse, 2));
checks.push(JSON.stringify(spreadSparse) === "[null,null,null]");

// A hole from growing .length past the backing elements slice.
const grown: any = [1, 2, 3];
grown.length = 6;
const spreadGrown: any = [...grown];
checks.push(spreadGrown.length === 6);
checks.push(present(spreadGrown, 3) && present(spreadGrown, 4) && present(spreadGrown, 5));
checks.push(JSON.stringify(spreadGrown) === "[1,2,3,null,null,null]");

// A literal elision - not (yet) a real hole here, but must spread as a
// present undefined regardless.
const elided: any = [1, , 3];
const spreadElided: any = [...elided];
checks.push(spreadElided.length === 3);
checks.push(present(spreadElided, 1));
checks.push(JSON.stringify(spreadElided) === "[1,null,3]");

// A hole propagating through a function call's spread arguments.
function collect(...args: any[]): any[] {
  return args;
}
const collected = collect(...deleted);
checks.push(collected.length === 3);
checks.push(present(collected, 1));
checks.push(collected[1] === undefined);

// A hole propagating through a spread `new` call.
class Point {
  x: any;
  y: any;
  z: any;
  constructor(x: any, y: any, z: any) {
    this.x = x;
    this.y = y;
    this.z = z;
  }
}
const p = new Point(...deleted);
checks.push(p.x === 1 && p.y === undefined && p.z === 3);

// Regular (non-hole) elements are unaffected.
checks.push(JSON.stringify([...[10, 20, 30]]) === "[10,20,30]");

checks.every((c) => c === true);
