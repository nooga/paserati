// expect: true|false|true|false|true|true|false|true|true|false
// UnicodeSets mode (the v flag): nested classes, intersection, subtraction,
// \q{...} string members and properties of strings inside classes.
let out: boolean[] = [];
out.push(/^[\p{L}--[a-z]]+$/v.test("ÀB"));
out.push(/^[\p{L}--[a-z]]+$/v.test("a"));
out.push(/^[[a-z]&&[aeiou]]$/v.test("e"));
out.push(/^[[a-z]&&[aeiou]]$/v.test("b"));
out.push(/^[\q{abc|d}x]+$/v.test("abcxd"));
out.push(/^[\w--_]+$/vi.test("ſK"));             // case folding applies before --
out.push(/^[\p{Emoji_Keycap_Sequence}--\q{0️⃣}]$/v.test("0️⃣"));
out.push(/^[\p{Emoji_Keycap_Sequence}\d]+$/v.test("1️⃣2"));
out.push(/^\p{Basic_Emoji}$/v.test("⌚"));
out.push(/^[]$/v.test(""));                      // the empty class matches nothing
out.join("|");
