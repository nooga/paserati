// expect: hello
// Test: !== undefined narrowing in if-statement and && expressions

interface Desc {
  get?: () => string;
  value?: string;
}

function read(d: Desc | undefined): string {
  if (d !== undefined) {
    // d should be narrowed to Desc
    if (typeof d.get === "function") {
      return d.get();
    }
    if (d.value !== undefined) {
      return d.value;
    }
  }
  return "none";
}

// Also test && chain with !== undefined
function check(d: Desc | undefined): boolean {
  return d !== undefined && d.value !== undefined;
}

const desc: Desc = { get: () => "hello" };
read(desc);
