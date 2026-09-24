// Valid numeric/string/template forms that the stricter lexer must still accept.
// expect: 1.5|8.5|10|255|31|0|512|7|\u0080|8|undefined
// no-typecheck

function tag(strings: TemplateStringsArray) {
  return strings[0];
}

function sloppy() {
  return [08.5, 0o10, "\200", "\8"];
}

const parts = [
  1.5,
  sloppy()[0],
  1_0,
  0xf_f,
  Number(0x1_fn),
  0,
  2e0_2 + 312,
  1.e1 - 3,
  sloppy()[2] === "\u0080" ? "\\u0080" : "bad",
  sloppy()[3],
  String(tag`\unicode`),
];
parts.join("|");
