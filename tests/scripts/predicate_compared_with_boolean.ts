// expect: true
// A type predicate's call compared with a boolean literal narrows like the
// call itself (is-plain-object's `isObject(o) === false`), its value is a
// boolean, and Object.prototype's members are read before a string index
// signature (`prot.hasOwnProperty` on a Record<string, unknown>).
function isObject(o: unknown): o is Record<string, unknown> {
  return Object.prototype.toString.call(o) === "[object Object]";
}

function isPlainObject(o: unknown): o is Record<string, unknown> {
  if (isObject(o) === false) return false;
  const ctor = o.constructor;
  if (ctor === undefined) return true;
  const prot: unknown = (ctor as { prototype: unknown }).prototype;
  if (false === isObject(prot)) return false;
  return prot.hasOwnProperty("isPrototypeOf") !== false;
}

const flag: boolean = isObject(1);
isPlainObject({ a: 1 }) && !flag;
