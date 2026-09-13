// expect: 5|9|1,2|9,8
// Regression test: a destructuring parameter pattern with a default value on
// a nested object/array pattern two or more levels deep - e.g.
// `{a: {b: {c} = {}} = {}}` - used to fail to compile with
// "unsupported nested pattern type: *parser.ObjectDestructuringAssignment"
// (or ArrayDestructuringAssignment for the array-pattern analog).
//
// Root cause: parseParameterDestructuringProperty parses a property's
// explicit target (`key: target`) via parseObjectLiteral/parseArrayLiteral,
// whose own property VALUES are parsed as plain expressions
// (parseExpression). One level deep, that's fine: `{b: {c} = {}}`'s `{c} = {}`
// is parsed as a generic *parser.AssignmentExpression (Left={c}, Right={}),
// which compileNestedObjectDeclaration already knew how to split into
// Target/Default. But when the LEFT side of that inner `=` is itself an
// object/array literal (`{} = {}` or `[x, y] = [1, 2]`, as opposed to
// `{c} = {}`), the parser's parseAssignmentExpression has a special case
// that produces a full *parser.ObjectDestructuringAssignment/
// ArrayDestructuringAssignment node instead (the node normally used for a
// *standalone* destructuring-assignment expression like `({a} = obj)`) -
// and nothing recognized that shape as a nested-pattern-with-default until
// compileNestedPatternDeclaration (compile_nested_declarations.go) and the
// checker's unwrapNestedPatternTarget (destructuring_nested.go) both gained
// cases for it.
//
// Also exercises: the pattern compiles and BINDS correctly, not just that it
// compiles - the fix must resolve `undefined -> default` for the nested
// pattern the same way a top-level default already does. Typechecked (no
// `// no-typecheck`), so the checker-side fix is exercised too, not just the
// compiler's.
function f({ a: { b: { c = 5 } = {} } = {} } = {}) {
  return c;
}

const results: string[] = [];
results.push(String(f())); // every level falls through to its default: c = 5
results.push(String(f({ a: { b: { c: 9 } } }))); // c provided explicitly: c = 9

// Array-pattern analog: a nested object property whose own value is an
// array pattern carrying a default (ArrayDestructuringAssignment, not
// ObjectDestructuringAssignment). Needs an explicit type annotation to
// actually reach the checker's strict destructuring-target dispatch for
// this shape - an un-annotated (inferred) parameter takes a more lenient
// inference path that doesn't exercise unwrapNestedPatternTarget's
// ArrayDestructuringAssignment case at all, so without the annotation this
// would pass even with that checker case missing.
function g(
  { a: { b: [x, y] = [1, 2] } = {} }: { a?: { b?: number[] } } = {}
) {
  return x + "," + y;
}
results.push(String(g())); // falls through to the array default: [1, 2]
results.push(String(g({ a: { b: [9, 8] } }))); // provided explicitly: [9, 8]

results.join("|");
