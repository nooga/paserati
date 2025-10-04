// Test Proxy.revocable functionality
const target = { x: 1 };
const handler = {
  get: function (target, prop, receiver) {
    return target[prop];
  },
};

const { proxy, revoke } = Proxy.revocable(target, handler);

console.log("proxy.x:", proxy.x);
revoke();
console.log("Proxy revoked");

try {
  console.log("Accessing revoked proxy:", proxy.x);
} catch (e) {
  console.log("Expected error after revoke:", e.message);
}

// expect: value
// expect: 1
