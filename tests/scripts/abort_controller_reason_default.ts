// expect: true
// AbortController.prototype.abort(reason?) and AbortSignal.abort(reason?) must
// treat an explicit `undefined` reason the same as an omitted one (DOM spec:
// "If reason is undefined, set reason to a new 'AbortError' DOMException"),
// not just an absent argument. Once the type signatures declared `reason` as
// an optional parameter, the compiler pads an omitted call with an explicit
// `undefined` argument, so a bare `len(args) > 0` check would wrongly treat
// `abort()` as if a real reason had been supplied.
const checks: boolean[] = [];

// AbortController.prototype.abort
const c1 = new AbortController();
c1.abort();
checks.push(String(c1.signal.reason).includes("AbortError"));

const c2 = new AbortController();
c2.abort(undefined);
checks.push(String(c2.signal.reason).includes("AbortError"));

const c3 = new AbortController();
c3.abort("custom reason");
checks.push(c3.signal.reason === "custom reason");

// AbortSignal.abort (static)
const s1 = AbortSignal.abort();
checks.push(String(s1.reason).includes("AbortError"));

const s2 = AbortSignal.abort(undefined);
checks.push(String(s2.reason).includes("AbortError"));

const s3 = AbortSignal.abort("custom sig reason");
checks.push(s3.reason === "custom sig reason");

checks.every((v) => v === true);
