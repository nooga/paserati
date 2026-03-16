// expect: all Array.isArray type guard tests passed
// Tests for Array.isArray as a type guard with narrowing

// ==========================================
// 1. Basic narrowing: union with array
// ==========================================

function joinOrReturn(x: string | string[]): string {
  if (Array.isArray(x)) {
    return x.join(",");
  }
  return x;
}

if (joinOrReturn(["a", "b"]) !== "a,b") throw "FAIL basic array branch";
if (joinOrReturn("hello") !== "hello") throw "FAIL basic string branch";

// ==========================================
// 2. Array.isArray with && chain
// ==========================================

function firstElement(x: string | number[]): number {
  if (Array.isArray(x) && x.length > 0) {
    return x[0];
  }
  return -1;
}

if (firstElement([42, 43]) !== 42) throw "FAIL && chain";
if (firstElement("nope") !== -1) throw "FAIL && chain string";

// ==========================================
// 3. Negated Array.isArray
// ==========================================

function negated(x: string | string[]): string {
  if (!Array.isArray(x)) {
    return x;
  }
  return x.join(",");
}

if (negated("solo") !== "solo") throw "FAIL negated string";
if (negated(["a", "b"]) !== "a,b") throw "FAIL negated array";

// ==========================================
// 4. Array.isArray with optional parameter
// ==========================================

function countItems(items?: string[]): number {
  if (Array.isArray(items)) {
    return items.length;
  }
  return 0;
}

if (countItems(["x", "y", "z"]) !== 3) throw "FAIL optional param with value";
if (countItems() !== 0) throw "FAIL optional param undefined";

// ==========================================
// 5. Array.isArray on optional chaining (repro 2 from bug)
// ==========================================

interface NodeMeta {
  verbs?: string[];
}

function getVerbs(meta?: NodeMeta): string {
  if (Array.isArray(meta?.verbs) && meta.verbs.length > 0) {
    return meta.verbs.join(",");
  }
  return "none";
}

if (getVerbs({ verbs: ["get", "post"] }) !== "get,post") throw "FAIL optional chaining isArray";
if (getVerbs({}) !== "none") throw "FAIL optional chaining isArray empty";
if (getVerbs() !== "none") throw "FAIL optional chaining isArray undefined";

// ==========================================
// 6. Array.isArray with number union
// ==========================================

function sumOrValue(x: number | number[]): number {
  if (Array.isArray(x)) {
    let sum = 0;
    for (const n of x) {
      sum = sum + n;
    }
    return sum;
  }
  return x;
}

if (sumOrValue([1, 2, 3]) !== 6) throw "FAIL number union array";
if (sumOrValue(42) !== 42) throw "FAIL number union scalar";

"all Array.isArray type guard tests passed";
