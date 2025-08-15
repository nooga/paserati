function deepEqual(
  a: any,
  b: any,
  cache?: Map<any, Map<any, number>>
): boolean {
  const EQUAL = 1;
  const NOT_EQUAL = -1;
  const UNKNOWN = 0;
  function isOptional(v: any) {
    return v == null;
  }
  function isPrimitiveEquatable(v: any) {
    const t = typeof v;
    return (
      t === "string" ||
      t === "number" ||
      t === "bigint" ||
      t === "boolean" ||
      t === "symbol" ||
      v instanceof String ||
      v instanceof Number ||
      v instanceof Boolean
    );
  }
  function tryStrict(a: any, b: any) {
    return a === b ? EQUAL : UNKNOWN;
  }
  function comparePrimitive(a: any, b: any) {
    if (a instanceof String) a = a.valueOf();
    if (b instanceof String) b = b.valueOf();
    if (a instanceof Number) a = a.valueOf();
    if (b instanceof Number) b = b.valueOf();
    return (
      tryStrict(a, b) ||
      (typeof a !== typeof b ? NOT_EQUAL : UNKNOWN) ||
      (typeof a === "number" && isNaN(a) && isNaN(b) ? EQUAL : NOT_EQUAL)
    );
  }
  function getCache(cache: Map<any, Map<any, number>>, left: any, right: any) {
    const oc = cache.get(left);
    const r = oc && oc.get(right);
    if (r) return r;
    const oc2 = cache.get(right);
    const r2 = oc2 && oc2.get(left);
    return r2 || UNKNOWN;
  }
  function setCache(
    cache: Map<any, Map<any, number>>,
    left: any,
    right: any,
    result: number
  ) {
    let oc = cache.get(left);
    if (!oc) cache.set(left, (oc = new Map()));
    oc.set(right, result);
    oc = cache.get(right);
    if (!oc) cache.set(right, (oc = new Map()));
    oc.set(left, result);
  }
  function compare(a: any, b: any, cmp: (a: any, b: any) => number) {
    const res = cmp(a, b);
    if (cache && (res === EQUAL || res === NOT_EQUAL))
      setCache(cache, a, b, res);
    return res;
  }
  function compareObjects(a: any, b: any) {
    console.log("enter compareObjects");
    if (!cache) cache = new Map();
    const hit = getCache(cache, a, b);
    if (hit !== UNKNOWN) {
      console.log("cache hit", hit);
      return hit;
    }
    setCache(cache, a, b, EQUAL);
    // struct compare via for-in
    const aKeys: string[] = [];
    for (const k in a) {
      aKeys.push(k);
    }
    const bKeys: string[] = [];
    for (const k in b) {
      bKeys.push(k);
    }
    console.log("keys lens", aKeys.length, bKeys.length);
    if (aKeys.length !== bKeys.length) return NOT_EQUAL;
    aKeys.sort();
    bKeys.sort();
    for (let i = 0; i < aKeys.length; i++) {
      if (deepEqual(aKeys[i], bKeys[i], cache) === NOT_EQUAL) return NOT_EQUAL;
      if (deepEqual(a[aKeys[i]], b[bKeys[i]], cache) === NOT_EQUAL)
        return NOT_EQUAL;
    }
    console.log("return EQUAL");
    return EQUAL;
  }
  function compareIf(
    a: any,
    b: any,
    test: (v: any) => boolean,
    cmp: (a: any, b: any) => number
  ) {
    const out = !test(a)
      ? !test(b)
        ? UNKNOWN
        : NOT_EQUAL
      : !test(b)
      ? NOT_EQUAL
      : compare(a, b, cmp);
    console.log("compareIf out", out, typeof out);
    return out;
  }
  const t1 = compareIf(
    a,
    b,
    isOptional,
    (x, y) => tryStrict(x, y) || NOT_EQUAL
  );
  console.log("t1", t1, typeof t1);
  const t2 = t1 || compareIf(a, b, isPrimitiveEquatable, comparePrimitive);
  console.log("t2", t2, typeof t2);
  const t3 =
    t2 ||
    compareIf(
      a,
      b,
      (v) => typeof v === "object" || typeof v === "function",
      compareObjects
    );
  console.log("t3", t3, typeof t3);
  const tri = t3 || NOT_EQUAL;
  console.log("tri", tri, typeof tri);
  return tri === EQUAL;
}

// Minimal repro cases
(function () {
  const a = {};
  const b = {};
  console.log("empty-objects", deepEqual(a, b));
})();

(function () {
  const a: any = {};
  const b: any = {};
  a.x = 1;
  b.x = 1;
  console.log("single-prop", deepEqual(a, b));
})();

(function () {
  const a: any = {};
  const b: any = {};
  a.x = {};
  b.x = {};
  console.log("nested-empty", deepEqual(a, b));
})();
