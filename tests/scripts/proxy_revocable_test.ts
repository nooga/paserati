// Test Proxy.revocable functionality
// expect: done
const target = { x: 1 };
const handler = {
  get: function (target, prop, receiver) {
    return target[prop];
  },
};

const { proxy, revoke } = Proxy.revocable(target, handler);

console.log("proxy.x:", proxy.x);
revoke();
console.log("Proxy revoked successfully");

"done";
