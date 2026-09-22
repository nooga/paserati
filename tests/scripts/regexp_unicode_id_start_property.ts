// expect: true
// \p{ID_Start} / \p{ID_Continue} are ECMAScript derived binary properties
// neither RE2 nor regexp2 knows by name (paserati#190). They're expanded to
// explicit ranges before compilation, on both the literal and constructor
// paths, inside and outside character classes, negated or not.

// The exact typebox/compile pattern that surfaced the gap.
const ident = /^[\p{ID_Start}_$][\p{ID_Continue}_$]*$/u;
const identCtor = new RegExp("^[\\p{ID_Start}_$][\\p{ID_Continue}_$]*$", "u");

const validIdents = ["hello", "_x", "$y", "héllo", "变量", "π2", "𝒳y", "á", "ª"];
const invalidIdents = ["1abc", "a-b", "", "a b", "é!", "a.b", "ⸯ"];

const checks: boolean[] = [];
for (const s of validIdents) {
  checks.push(ident.test(s) && identCtor.test(s));
}
for (const s of invalidIdents) {
  checks.push(!ident.test(s) && !identCtor.test(s));
}

// Bare (outside a class), negated, aliases, and a negated class.
checks.push(/^\p{ID_Start}$/u.test("a"));
checks.push(!/^\p{ID_Start}$/u.test("1"));
checks.push(/^\p{ID_Continue}$/u.test("1"));
checks.push(!/^\p{ID_Continue}$/u.test("-"));
checks.push(/^\P{ID_Start}$/u.test("1"));
checks.push(!/^\P{ID_Start}$/u.test("a"));
checks.push(/^[\P{ID_Start}]$/u.test("-"));
checks.push(!/^[\P{ID_Start}]$/u.test("z"));
checks.push(/^[^\p{ID_Start}]$/u.test("9"));
checks.push(!/^[^\p{ID_Start}]$/u.test("Z"));
checks.push(/^\p{IDS}\p{IDC}*$/u.test("a1"));

// ZWNJ is category Cf, but it IS part of Unicode's ID_Continue derived
// property as of the Unicode Character Database version bundled with the Go
// toolchain pinned in go.mod (see unicode.Other_ID_Continue - this is a
// per-Go-version snapshot, so it can differ across Go releases; verify with
// `go run ./cmd/paserati --no-typecheck -e '...'` under the pinned toolchain
// before assuming this assertion is stale). The second check below shows
// explicitly unioning in \u200c\u200d is (on the current toolchain)
// redundant but still correct.
checks.push(ident.test("x\u200c"));
checks.push(/^[\p{ID_Start}_$][\p{ID_Continue}_$\u200c\u200d]*$/u.test("x\u200c"));

// Unrelated property names are left to the engines and still work.
checks.push(/^\p{L}+$/u.test("abc"));
checks.push(/^\p{Lu}$/u.test("A") && !/^\p{Lu}$/u.test("a"));

// Combined with lookahead so the regexp2 path is exercised as well.
checks.push(/^(?=\p{ID_Start})\w+$/u.test("abc"));
checks.push(!/^(?=\p{ID_Start})\w+$/u.test("1abc"));
checks.push(new RegExp("^(?!\\p{ID_Start})\\p{ID_Continue}+$", "u").test("123"));

// Source is reported as written, not as expanded.
checks.push(ident.source === "^[\\p{ID_Start}_$][\\p{ID_Continue}_$]*$");

checks.every((c) => c);
