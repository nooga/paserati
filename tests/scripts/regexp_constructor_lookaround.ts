// expect: true
// paserati#172: RegExp's constructor form (new RegExp("...")) used to
// reject every lookaround construct - (?=, (?!, (?<=, (?<! - even though
// the identical pattern written as a literal (/.../) already worked via
// regexp2's fallback. Real JS never enforces this asymmetry: a dynamically
// built pattern (the common case for pattern-matching libraries like
// minimatch, which is what this blocked) must behave the same as a
// literal with the same source.
const negLookahead = new RegExp("a(?!b)");
const posLookahead = new RegExp("a(?=b)");
const posLookbehind = new RegExp("(?<=a)b");
const negLookbehind = new RegExp("(?<!a)b");

negLookahead.test("ac") &&
  !negLookahead.test("ab") &&
  posLookahead.test("ab") &&
  !posLookahead.test("ac") &&
  posLookbehind.test("ab") &&
  !posLookbehind.test("cb") &&
  negLookbehind.test("cb") &&
  !negLookbehind.test("ab") &&
  // Constructor and literal must agree for the same source.
  new RegExp("a(?!b)").test("ac") === /a(?!b)/.test("ac");
