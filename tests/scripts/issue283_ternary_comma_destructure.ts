// expect: true
// paserati#283: parseArrayDestructuringAssignment/parseObjectDestructuringAssignment
// parsed their right-hand side at LOWEST precedence instead of ARG_SEPARATOR
// (the same precedence parseAssignmentExpression's regular `x = value` path
// already correctly uses right above it). LOWEST is below COMMA, so
// `[a, b] = c, d` swallowed the comma operator into the destructuring's own
// RHS - parsing as `[a, b] = (c, d)` (destructuring the comma expression's
// *result*) instead of the correct `([a, b] = c), d` (an assignment whose
// RHS is `c`, followed by a separate, discarded operand `d`).
//
// Most visibly wrong when the destructuring assignment is itself a
// ternary's alternate branch, inside a comma expression - the exact shape
// found in @babel/parser's own bundled `getParser`, whose miscompilation
// under paserati silently emptied the enabled-plugin set for every
// TypeScript-specific syntax family.
const checks: boolean[] = [];

// --- Case 1: exactly the reported shape - ternary alternate is an array
// destructuring assignment, inside a comma expression ---
// NOTE: the alternate branch below is DELIBERATELY left unparenthesized
// (`[key, val] = item`, not `([key, val] = item)`) - wrapping it in parens
// bounds its RHS parse at the closing paren regardless of the precedence
// bug, which would silently stop exercising #283 at all.
{
  let key: any, val: any;
  const item: any = ["typescript", {}];
  "string" == typeof item ? key = item : [key, val] = item, 1;
  checks.push(key === "typescript" && val && typeof val === "object");
}

// --- Case 2: the comma's second operand actually does something
// observable (matches the issue's second, non-crashing variant) ---
{
  let key: any, val: any;
  const seen = new Map<string, any>();
  const item: any = ["typescript", {}];
  "string" == typeof item
    ? key = item
    : [key, val] = item,
    seen.has(key) || seen.set(key, val || {});
  checks.push(
    key === "typescript" &&
      val &&
      typeof val === "object" &&
      seen.has("typescript")
  );
  // The source array must be left untouched.
  checks.push(JSON.stringify(item) === '["typescript",{}]');
}

// --- Case 3: object destructuring assignment in the identical position ---
{
  let key: any, val: any;
  const item: any = { key: "typescript", val: {} };
  "string" == typeof item ? key = item : { key, val } = item, 1;
  checks.push(key === "typescript" && val && typeof val === "object");
}

// --- Controls: each must still behave correctly, unchanged by the fix ---

// A plain (non-destructuring) assignment must still stop its RHS at the
// comma, exactly like before.
{
  let a: any;
  a = 5, 10;
  checks.push(a === 5);
}

// A parenthesized destructuring assignment as a comma operand (already
// correct pre-fix - parens force LOWEST regardless of this precedence)
// must keep working.
{
  let a: any, b: any;
  const x = (([a, b] = [1, 2]), 99);
  checks.push(a === 1 && b === 2 && x === 99);
}

// The ternary alone, with no comma-continuation, must still work.
{
  let key: any, val: any;
  const item: any = ["typescript", {}];
  "string" == typeof item ? (key = item) : ([key, val] = item);
  checks.push(key === "typescript" && val && typeof val === "object");
}

// Guard against a check silently never running (e.g. a parse/compile error
// swallowed earlier) making checks.every() pass vacuously on a short array.
checks.length === 7 && checks.every((c) => c);
