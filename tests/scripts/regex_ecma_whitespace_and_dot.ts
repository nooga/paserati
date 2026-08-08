// ECMAScript's \s is WhiteSpace ∪ LineTerminator, which includes U+00A0,
// U+FEFF and every Zs; its `.` excludes the four LineTerminators. Go's RE2
// agrees on neither, and it ACCEPTS such a pattern rather than rejecting it,
// so the wrong semantics used to reach the matcher silently.
const checks: boolean[] = [
    // \s must reach past ASCII whitespace.
    "a b".replace(/\s+/g, "") === "ab",
    "a b".replace(/\s+/g, "") === "ab",
    "a﻿b".replace(/\s+/g, "") === "ab",
    // ...without losing the ASCII members.
    "a \t\nb".replace(/\s+/g, "") === "ab",
    // \S is its exact complement, inside a character class too — RE2 has no
    // set complement there, so that case is spelled out as ranges.
    "a b".replace(/\S+/g, "X") === "X X",
    "a b".replace(/[\S]+/g, "X") === "X X",
    // `.` must not match a LineTerminator, but must match everything under s.
    "a b".replace(/./g, "X") === "X X",
    "a b".replace(/./gs, "X") === "XXX",
    // An inline (?s:) modifier widens `.` for its own scope; the rewrite has
    // to leave those patterns alone rather than clamp them.
    /(?s:.)/.test("\n") === true,
];

checks.every((c: boolean): boolean => c);
// expect: true
