// Test comparison operators with string lexicographic comparison and NaN handling
// expect: pass

// Test NaN comparisons - all should be false
if (NaN >= 0) {
  throw new Error('NaN >= 0 should be false');
}
if (NaN <= 0) {
  throw new Error('NaN <= 0 should be false');
}
if (NaN > 0) {
  throw new Error('NaN > 0 should be false');
}
if (NaN < 0) {
  throw new Error('NaN < 0 should be false');
}

// Test string lexicographic comparison (use any to bypass type checker)
const s1: any = "x";
const s2: any = "x ";
if (s1 >= s2) {
  throw new Error('"x" >= "x " should be false (shorter comes first)');
}

const s3: any = "xy";
const s4: any = "xx";
if (!(s3 > s4)) {
  throw new Error('"xy" > "xx" should be true');
}

// Test null/undefined (use any)
const n: any = null;
const u: any = undefined;
if (n >= u) {
  throw new Error('null >= undefined should be false');
}

// Test object with valueOf
const obj: any = {
  valueOf() {
    return 10;
  }
};
if (!(obj > 5)) {
  throw new Error('Object with valueOf() returning 10 should be > 5');
}

'pass';
