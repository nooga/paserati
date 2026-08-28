// charCodeAt / codePointAt / bracket indexing / at() address UTF-16 code units.
//
// Companion to string_utf16_indexing.ts. These four went through
// vm.StringToUTF16, which re-decoded the whole string on every call — O(n^2)
// for a scanner loop. They now share one code-unit path (see
// pkg/vm/string_utf16.go) with an ASCII fast path and a classification cache.
// Bracket indexing in particular changed semantics: it used to index by Unicode
// code *point* (`[]rune`), so `s[i]` past an astral character was off by one and
// an astral character came back whole instead of as its lead surrogate.

const astral = "a😀b"; // code units: 'a', D83D, DE00, 'b'  — 4 units, not 3

const len = astral.length === 4;

// charCodeAt exposes the raw code units, including each half of the pair.
const cc0 = astral.charCodeAt(0) === 0x61;
const cc1 = astral.charCodeAt(1) === 0xd83d;
const cc2 = astral.charCodeAt(2) === 0xde00;
const cc3 = astral.charCodeAt(3) === 0x62;
const ccOOB = Number.isNaN(astral.charCodeAt(4));

// codePointAt combines the pair at the lead surrogate, returns the bare unit
// at the trail.
const cp0 = astral.codePointAt(0) === 0x61;
const cp1 = astral.codePointAt(1) === 0x1f600;
const cp2 = astral.codePointAt(2) === 0xde00;
const cpOOB = astral.codePointAt(4) === undefined;

// Bracket indexing is by code unit: s[1] is the lead surrogate alone, and the
// trailing 'b' sits at index 3, not 2.
const u1 = astral[1] as string;
const br1 = u1.length === 1 && u1.charCodeAt(0) === 0xd83d;
const br3 = astral[3] === "b";
const brOOB = astral[4] === undefined;

// at() takes the same units and supports negative indices.
const at1 = (astral.at(1) as string).charCodeAt(0) === 0xd83d;
const atNeg = astral.at(-1) === "b";
const atOOB = astral.at(9) === undefined;

// Plain ASCII stays on the fast path and behaves normally.
const ascii = "hello";
const asciiOk =
  ascii.length === 5 &&
  ascii.charCodeAt(1) === 0x65 &&
  ascii[4] === "o" &&
  ascii.at(-2) === "l" &&
  ascii.codePointAt(0) === 0x68;

// A scanner-style loop (the tsc hot path) sums every code unit.
let sum = 0;
for (let i = 0; i < astral.length; i++) sum += astral.charCodeAt(i);
const loop = sum === 0x61 + 0xd83d + 0xde00 + 0x62;

len && cc0 && cc1 && cc2 && cc3 && ccOOB && cp0 && cp1 && cp2 && cpOOB &&
  br1 && br3 && brOOB && at1 && atNeg && atOOB && asciiOk && loop;

// expect: true
