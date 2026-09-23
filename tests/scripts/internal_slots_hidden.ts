// expect: ["message","stack"]["message","stack"]["errors","message","stack"]~[][]["0","1","length"][][][]~TypeError,TypeError,TypeError,TypeError,TypeError,TypeError,[object Object]~false,false,false,0,{},{}~1234,1234,5,hi,[object Error],true,4,"s",false~[object Error],7,10,["message","stack"],[]
// skip-typecheck
// paserati#525: internal slots ([[ErrorData]], [[PrimitiveValue]], a Date's time
// value) were ordinary non-enumerable properties - visible to Reflect.ownKeys,
// getOwnPropertyNames, in and hasOwn, and forgeable:
// Date.prototype.getTime.call({__timestamp__: 5}) returned 5. They are internal
// slots now, invisible to every property API.
const keys = (v) => JSON.stringify(Reflect.ownKeys(v).map(String).sort());
const out = [];
out.push(keys(new Error("x")) + keys(new TypeError("y")) + keys(new AggregateError([], "z")));
out.push(keys(new Date(0)) + keys(new Number(1)) + keys(new String("ab")) + keys(new Boolean(true)) + keys(Object(Symbol("s"))) + keys(Object(1n)));
const t = (f) => { try { return String(f()); } catch (e) { return e.constructor.name; } };
out.push([t(() => Date.prototype.getTime.call({ __timestamp__: 5 })), t(() => Number.prototype.valueOf.call({ "[[PrimitiveValue]]": 7 })),
  t(() => String.prototype.toString.call({ "[[PrimitiveValue]]": "q" })), t(() => Boolean.prototype.valueOf.call({ "[[PrimitiveValue]]": true })),
  t(() => Symbol.prototype.valueOf.call({ "[[PrimitiveValue]]": Symbol.iterator })), t(() => BigInt.prototype.valueOf.call({ "[[PrimitiveValue]]": 1n })),
  Object.prototype.toString.call({ "[[ErrorData]]": true })].join());
out.push([Object.hasOwn(new Error("x"), "[[ErrorData]]"), "__timestamp__" in new Date(0), "[[PrimitiveValue]]" in new Number(1),
  Object.getOwnPropertyNames(new Date(5)).length, JSON.stringify(Object.assign({}, new Number(3), new Date(0))), JSON.stringify({ ...new Boolean(true) })].join());
// the slots still work
const d = new Date(1234); const n = new Number(4); const e = new RangeError("r");
out.push([d.getTime(), new Date(d).getTime(), n + 1, String(new String("hi")), Object.prototype.toString.call(e), e instanceof Error, JSON.stringify(n), JSON.stringify(new String("s")), JSON.stringify(new Boolean(false))].join());
class MyErr extends Error {} class MyDate extends Date {} class MyNum extends Number {}
out.push([Object.prototype.toString.call(new MyErr("m")), new MyDate(7).getTime(), new MyNum(5) * 2, keys(new MyErr("m")), keys(new MyDate(1))].join());
out.join("~");
