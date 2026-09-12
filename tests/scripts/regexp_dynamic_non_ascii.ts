// expect: true
// paserati#425: new RegExp(str) with a non-ASCII \xNN-escaped char (or a
// range of them) previously threw "invalid UTF-8" because the JS string
// itself was corrupted by the lexer bug fixed alongside this (see
// string_x_escape_non_ascii.ts) - the pattern handed to Go's regexp package
// contained a lone invalid byte. A regex *literal* with the same escapes
// compiled without error but silently matched wrong, for the same reason:
// the test string it was matched against was equally corrupted.
const re1 = new RegExp("[\xaa]");
const re2 = /[\xc0-\xd6]/;

re1.test("\xaa") && re2.test("\xc0") && !re2.test("\xd7");
