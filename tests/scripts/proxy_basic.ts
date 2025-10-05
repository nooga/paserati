// Test basic Proxy functionality
// expect: done

const target = { x: 1 };
const handler = {
  get(t, p) {
    if (p === "x") return t[p] * 2;
    return t[p];
  }
};

const proxy = new Proxy(target, handler);
console.log(proxy.x); // 2

// Test Proxy.revocable
const { proxy: p2, revoke } = Proxy.revocable({}, {});
console.log(typeof p2); // object
console.log(typeof revoke); // function
revoke();

// Test has trap
const target2 = { a: 1 };
const handler2 = {
  has(t, p) {
    return p === "secret" || p in t;
  }
};
const proxy2 = new Proxy(target2, handler2);
console.log("secret" in proxy2); // true
console.log("a" in proxy2); // true
console.log("b" in proxy2); // false

// Test prototype inheritance
console.log(Object.getPrototypeOf(proxy) === Object.prototype); // true

"done";
