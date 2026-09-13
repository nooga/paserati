// paserati#454 - compound assignment targeting a member expression as an
// array element (the exact shape found in handlebars' bundled Jison lexer:
// `[this.offset, this.offset += this.yyleng]`).
// expect: 1,8
// no-typecheck

const obj = { offset: 5 };
[1, obj.offset += 3].join(",");
