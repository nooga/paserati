// expect: 84
let x: string | number = 42;
let result = 0;
if (typeof x === "number") {
  result = x * 2;
} else {
  result = 0;
}
if (typeof x === "string") {
  result = 1;
} else {
  result = x * 2;
}
result;
