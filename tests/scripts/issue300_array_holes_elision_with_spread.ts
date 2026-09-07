// expect: true
// paserati#300 follow-up: an elision that coexists with a spread element in
// the same array literal (e.g. [1,,...xs,,2]) went through a different
// compiler path - compileArrayLiteralWithSpread, used whenever the literal
// contains at least one SpreadElement - which was untouched by the original
// #300 fix (compileArrayLiteralSimple only) and still materialized those
// elisions as a real `undefined` value instead of a hole.
//
// Root cause: that path builds the result incrementally, appending each
// element via OpArraySpread (wrapping a plain element in a synthetic
// 1-element array first). OpArraySpread reads its source through the real
// ECMAScript iterator protocol, which resolves a Hole back into Undefined -
// so stuffing a Hole into the synthetic carrier array and spreading it
// would never survive the round trip. Fixed with a new opcode,
// OpArrayAppendRaw, that appends one register's raw value directly (a
// plain ArrayObject.Append - the same primitive `delete`/`new Array(n)`
// already use), bypassing any iterator/property-set semantics; a genuine
// spread element (`...xs`) still goes through OpArraySpread as before,
// since THAT must resolve a hole in its own source array to Undefined per
// spec - only the plain-element/elision case changed.
const checks: boolean[] = [];

const xs = [10, 20];
const mixed: any = [1, , ...xs, , 2];
checks.push(mixed.length === 6);
checks.push(JSON.stringify(mixed) === "[1,null,10,20,null,2]");

// Elided positions: real holes, absent everywhere.
checks.push((1 in mixed) === false);
checks.push((4 in mixed) === false);
checks.push(mixed.hasOwnProperty(1) === false);
checks.push(mixed.hasOwnProperty(4) === false);
checks.push(Object.keys(mixed).join(",") === "0,2,3,5");
checks.push(Object.getOwnPropertyNames(mixed).join(",") === "0,2,3,5,length");

// Regular and spread-contributed positions: present, unaffected.
checks.push((0 in mixed) === true && mixed[0] === 1);
checks.push((2 in mixed) === true && mixed[2] === 10);
checks.push((3 in mixed) === true && mixed[3] === 20);
checks.push((5 in mixed) === true && mixed[5] === 2);

const visited: number[] = [];
mixed.forEach((v: any, i: number) => visited.push(i));
checks.push(visited.join(",") === "0,2,3,5");

// Elision immediately before a spread, with nothing before it.
const leading: any = [, ...xs];
checks.push(leading.length === 3);
checks.push((0 in leading) === false);
checks.push((1 in leading) === true && leading[1] === 10);
checks.push((2 in leading) === true && leading[2] === 20);

// Elision immediately after a spread, with nothing after it.
const trailing: any = [...xs, ,];
checks.push(trailing.length === 3);
checks.push((0 in trailing) === true && (1 in trailing) === true);
checks.push((2 in trailing) === false);

// Multiple consecutive elisions surrounding a spread.
const both: any = [, , ...xs, , ,];
checks.push(both.length === 6);
checks.push((0 in both) === false && (1 in both) === false);
checks.push((2 in both) === true && (3 in both) === true);
checks.push((4 in both) === false && (5 in both) === false);
checks.push(Object.keys(both).join(",") === "2,3");

// Note: a spread whose SOURCE array has its own real hole (e.g.
// `[...[100,,300]]`) is a separate, still-open gap (spread reads its
// source's elements raw instead of resolving a hole to undefined per the
// iterator protocol) - untouched by this fix, which only changes how a
// literal's OWN elision is appended, not how OpArraySpread reads a spread
// argument. Not asserted here; tracked separately.

// A real `undefined` element next to a spread must NOT become a hole.
const real: any = [1, undefined, ...xs];
checks.push((1 in real) === true);
checks.push(real.hasOwnProperty(1) === true);
checks.push(Object.keys(real).join(",") === "0,1,2,3");

checks.every((c) => c === true);
