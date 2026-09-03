// expect: true
// paserati#218: new RegExp() with a numbered backreference (\1) should fall
// back to regexp2 the same way a regex literal does.
const re = new RegExp("(a)\\1");
re.test("aa") === true && re.test("ab") === false;
