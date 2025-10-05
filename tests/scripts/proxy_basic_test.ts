// Test basic Proxy functionality
// expect: done
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
console.log("target after set:", target);

"done";
