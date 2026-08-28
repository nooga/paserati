// const object destructuring with defaults inside a function must initialize
// the binding, not assign after DefineConstTDZ (which throws TypeError).
function pick(fields) {
  const { major = 5, minor = 8 } = fields;
  return major + "." + minor;
}
pick({}) + " " + pick({ major: 1 }) + " " + pick({ major: 9, minor: 2 });
// expect: 5.8 1.8 9.2
