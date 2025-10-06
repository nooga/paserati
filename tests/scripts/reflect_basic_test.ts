// Test basic Reflect functionality
// expect: done
const obj = { x: 1, y: 2 };

// Reflect.get
console.log("get x:", Reflect.get(obj, "x"));

// Reflect.set
console.log("set z:", Reflect.set(obj, "z", 3));

// Reflect.has
console.log("has z:", Reflect.has(obj, "z"));

// Reflect.deleteProperty
console.log("delete y:", Reflect.deleteProperty(obj, "y"));

// Reflect.ownKeys
const keys = Reflect.ownKeys(obj);
console.log("keys:", keys.join(","));

// Reflect.getPrototypeOf
const proto = Reflect.getPrototypeOf(obj);
console.log("has prototype:", proto !== null);

// Reflect with Proxy
const target = { a: 1 };
const handler = {
  get(t, p) {
    console.log("proxy get:", p);
    return Reflect.get(t, p);
  },
};
const proxy = new Proxy(target, handler);
console.log("proxy.a:", proxy.a);

"done";
