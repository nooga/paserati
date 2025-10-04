// Test basic Proxy functionality
const target = { x: 1, y: 2 };
const handler = {
  get: function (target, prop, receiver) {
    console.log(`Getting property ${prop}`);
    return target[prop];
  },
  set: function (target, prop, value, receiver) {
    console.log(`Setting property ${prop} to ${value}`);
    target[prop] = value;
    return true;
  },
};

const proxy = new Proxy(target, handler);

console.log("proxy.x:", proxy.x);
proxy.z = 3;
console.log("target:", target);

try {
  const { proxy: p, revoke } = Proxy.revocable(target, handler);
  console.log("Revocable proxy created");
  console.log("p.x:", p.x);
  revoke();
  console.log("Proxy revoked");
  try {
    p.x;
  } catch (e) {
    console.log("Expected error after revoke:", e.message);
  }
} catch (e) {
  console.log("Error:", e.message);
}
