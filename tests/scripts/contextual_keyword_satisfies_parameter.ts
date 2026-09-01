// expect: 33
// paserati#164 (follow-up): `satisfies` is a contextual keyword just like
// `as`, but isContextualKeywordAsIdent - the check parseFunctionParameters/
// parseParameterList use to accept a contextual keyword as a parameter
// name - was missing it, so `satisfies` (unlike `as`) failed to parse as a
// function or arrow parameter name specifically, in both declaration and
// arrow-function form.
function f(satisfies: number): number {
  return satisfies;
}
const g = (satisfies: number) => satisfies;
// Also cover satisfies as a non-first parameter, after a comma.
function h(a: number, satisfies: number): number {
  return a + satisfies;
}
const k = (a: number, satisfies: number) => a + satisfies;
f(9) + g(7) + h(1, 9) + k(1, 6);
