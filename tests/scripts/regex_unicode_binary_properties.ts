// expect: true|true|false|true|false|true|true|true|false|true|true|true|true|true|true|true|true
// Issue #196: \p{Default_Ignorable_Code_Point} - and, more generally, every
// ECMAScript binary Unicode property Go's unicode tables can express - is
// expanded into explicit ranges before either regex engine sees it, on both
// the literal and the RegExp-constructor path. Previously only ID_Start and
// ID_Continue were handled; RE2 knows no binary properties at all and
// regexp2 only a subset under canonical names.
let out: boolean[] = [];

// Default_Ignorable_Code_Point, its DI alias, and the \P negation.
out.push(/^\p{Default_Ignorable_Code_Point}$/u.test("​")); // ZERO WIDTH SPACE
out.push(new RegExp("^\\p{DI}$", "u").test("­"));          // SOFT HYPHEN
out.push(/^\p{DI}$/u.test("a"));
out.push(/^\P{DI}$/u.test("a"));
// U+0605 is Cf but Prepended_Concatenation_Mark, so it is excluded.
out.push(/^\p{DI}$/u.test("؅"));
// U+061C ARABIC LETTER MARK is Cf and stays in.
out.push(/^\p{DI}$/u.test("؜"));

// Derived properties from DerivedCoreProperties.txt.
out.push(/^\p{Alphabetic}+$/u.test("héllo"));
out.push(new RegExp("^\\p{Lower}+$", "u").test("abc"));
out.push(/^\p{Uppercase}$/u.test("a"));
out.push(/^\p{Math}$/u.test("+"));
out.push(/^\p{Cased}+$/u.test("aB"));

// Table properties under their UCD aliases, on the constructor path where RE2
// used to reject every one of them.
out.push(new RegExp("^\\p{space}$", "u").test(" "));
out.push(new RegExp("^\\p{AHex}+$", "u").test("0aF"));
out.push(new RegExp("^\\p{Dash}$", "u").test("-"));

// The ECMAScript-only names.
out.push(/^\p{Any}+$/u.test("a\u{10FFFF}"));
out.push(/^\p{ASCII}+$/u.test("abc"));
out.push(/^\p{Assigned}$/u.test("a"));

out.join("|");
