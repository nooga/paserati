// expect: Infinity-NaN-true
// Test string to number conversion edge cases
// Per ECMAScript spec: "Infinity" -> Infinity, "INFINITY" -> NaN

let a = +"Infinity";
let b = +"INFINITY";
let c = +"-Infinity";

a + "-" + b + "-" + (c === -Infinity)
