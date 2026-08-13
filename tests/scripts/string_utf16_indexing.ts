// String methods index by UTF-16 code unit, not by UTF-8 byte.
//
// `length`, `charAt` and `split("")` already went through the code-unit path
// while `substring`, `slice`, `substr`, `indexOf`, `lastIndexOf`, `startsWith`
// and `endsWith` sliced the underlying Go string by byte. One non-ASCII
// character was enough to split it: "abcédef".substring(4) started inside `é`
// and returned its continuation byte 0xA9 as a character.
//
// This is what made Octane's RegExp workload fail its own checksum — its
// computeInputVariants rebuilds a string around one rotated character with
// substring, so every variant of a non-ASCII input came out short.

const s = "abcédef"; // 7 code units, 8 UTF-8 bytes

// Slicing lands on character boundaries, and nothing leaks a partial byte.
const sub = s.substring(4) === "def";
const subRange = s.substring(2, 5) === "céd";
const sliced = s.slice(4) === "def";
const slicedNeg = s.slice(-3) === "def";
const substr = s.substr(4, 2) === "de";

// Indices are reported in the same units the caller passed in.
const idx = s.indexOf("d") === 4;
const lastIdx = s.lastIndexOf("f") === 6;
const starts = s.startsWith("d", 4) === true;
const ends = s.endsWith("é", 4) === true;

// The methods that were already correct stay correct.
const len = s.length === 7;
const at = s.charAt(4) === "d";
const parts = s.split("").length === 7;

// An astral character occupies two code units, so indices past it shift by two.
const astral = "a😀b";
const astralLen = astral.length === 4;
const astralTail = astral.substring(3) === "b";

sub && subRange && sliced && slicedNeg && substr && idx && lastIdx && starts &&
  ends && len && at && parts && astralLen && astralTail;

// expect: true
