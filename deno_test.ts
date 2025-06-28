function isString(x: any): x is string {
  return true; // Simplified for testing
}

function isNumber(x: any): x is number {
  return true; // Simplified for testing
}

let value: any = 42;

if (isNumber(value)) {
  let num: number = value;
  console.log("number: " + num);
} else {
  console.log("not a number");
}