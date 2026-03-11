// expect: b
// Test: ||-chain equality narrowing narrows to union of literal types
// if (x === "a" || x === "b" || x === "c") should narrow x to "a" | "b" | "c"

function classify(x: unknown): "a" | "b" | "c" | "other" {
  if (x === "a" || x === "b" || x === "c") {
    return x;  // x narrowed to "a" | "b" | "c"
  }
  return "other";
}

classify("b");
