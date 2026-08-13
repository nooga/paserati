// Regex-reported indices are UTF-16 code unit offsets, not UTF-8 byte offsets.
//
// Companion to string_utf16_indexing.ts. The String methods were fixed first;
// every index the regex path hands back went on being a byte offset, so on a
// string holding one `é` the match was reported one past where it happened —
// and `lastIndex` advanced by a number the caller could not use to slice.
//
// `index` is read through a helper because the checker types a match result as
// `null | string[]`, where `.index` is not a member. That is its own gap, and
// narrowing it here would only hide what this test is about.

const s = "abcédef"; // 7 code units, 8 UTF-8 bytes; 'd' sits at 4

function indexOfMatch(m: any): number {
  return m === null ? -1 : (m.index as number);
}

const search = s.search(/d/) === 4;
const matchIdx = indexOfMatch(s.match(/d/)) === 4;

const re = /d/g;
const execIdx = indexOfMatch(re.exec(s)) === 4;
const lastIdx = re.lastIndex === 5;

// Walking a global regex must visit every code unit exactly once, with no gap
// where the multi-byte character sits.
const all: number[] = [];
const re2 = /./g;
let x: any = re2.exec(s);
while (x !== null) {
  all.push(x.index as number);
  x = re2.exec(s);
}
const walk = all.join(",") === "0,1,2,3,4,5,6";

// The offset handed to a replacer callback is the same coordinate.
let seen = -1;
s.replace(/d/, function (mm: string, off: number) {
  seen = off;
  return mm;
});
const replaceOff = seen === 4;

// ...and the replacement itself still lands in the right place.
const replaced = s.replace(/d/, "X") === "abcéXef";

search && matchIdx && execIdx && lastIdx && walk && replaceOff && replaced;

// expect: true
