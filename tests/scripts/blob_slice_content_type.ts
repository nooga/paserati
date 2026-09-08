// expect: true

// Omitted contentType must default to "" (empty string), not inherit the
// parent Blob's type. See https://w3c.github.io/FileAPI/#slice-method-algo
const withType = new Blob(["abcdef"], { type: "text/plain" });
const sliced1 = withType.slice(0, 3);
const check1 = sliced1.type === "";

// Explicit contentType is still respected.
const sliced2 = withType.slice(0, 3, "text/html");
const check2 = sliced2.type === "text/html";

// Parent with no type at all.
const withoutType = new Blob(["abcdef"]);
const sliced3 = withoutType.slice();
const check3 = sliced3.type === "";

check1 && check2 && check3;
