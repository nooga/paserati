// Future: How else branch narrowing will work with union types
// This doesn't work yet but shows where we're heading!
// expect: ok

/*
// This is what we'll be able to do once we have union types:

function processValue(x: string | number): void {
    if (typeof x === "string") {
        // x is narrowed to string here
        console.log("String length:", x.length);
        console.log("Uppercase:", x.toUpperCase());
    } else {
        // x is narrowed to number here (the only remaining possibility)
        console.log("Number doubled:", x * 2);
        console.log("Is positive:", x > 0);
    }
}

processValue("hello");  // Takes string path
processValue(42);       // Takes number path

// More complex example:
function handleValue(x: string | number | boolean): string {
    if (typeof x === "string") {
        return "String: " + x;
    } else if (typeof x === "number") {
        return "Number: " + x.toString();
    } else {
        // x is narrowed to boolean here (only remaining type)
        return "Boolean: " + (x ? "true" : "false");
    }
}
*/

console.log("This is a vision of future union type narrowing!");
console.log("For now, we have solid unknown type narrowing working!");

// What works today:
let x: unknown = "test";
if (typeof x === "string") {
  console.log("Current: string length =", x.length);
} else {
  console.log("Current: not a string, still unknown");
}

("ok");
