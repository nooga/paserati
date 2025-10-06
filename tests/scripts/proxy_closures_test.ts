// Test: Proxy traps with closures work correctly
// expect: done
class Handler {
  private accessCount = 0;

  makeProxy(target: object) {
    const self = this;
    return new Proxy(target, {
      get(t, k) {
        self.accessCount++;
        return Reflect.get(t, k);
      },
      set(t, k, v) {
        self.accessCount++;
        return Reflect.set(t, k, v);
      },
    });
  }

  getCount() {
    return this.accessCount;
  }
}

const handler = new Handler();
const proxy = handler.makeProxy({ x: 1, y: 2 });

console.log("x =", proxy.x);
console.log("y =", proxy.y);
proxy.x = 10;
console.log("count:", handler.getCount());

"done";
