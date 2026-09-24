// expect: true|true|true|true|true|true|true|false|true
// Character escapes reach the engines as the characters they denote, so an
// escaped syntax character stays literal and escapes the engines spell
// differently (or not at all) still mean what ECMAScript says.
let out: boolean[] = [];
out.push(/\u{3f}/u.test("?"));        // not a quantifier once decoded
out.push(/\u{000000003f}/u.test("?")); // any number of leading zeros
out.push(/\u002A/.test("*"));
out.push(/[\u005D]/.test("]"));
out.push(/\cJ/.test("\n"));            // control escape
out.push(/\101/.test("A"));            // Annex B legacy octal
out.push(/^[\ud834\udf06]$/u.test("\ud834\udf06")); // escaped surrogate pair
out.push(/a\u{2}/.test("aa"));        // without u: u repeated twice, not U+0002
out.push(/\x2e/.test("."));
out.join("|");
