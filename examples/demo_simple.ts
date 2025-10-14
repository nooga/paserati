// Simple demo
const state = new Proxy(
  { count: 0 },
  {
    get(obj, prop) {
      console.log("get", prop);
      return Reflect.get(obj, prop);
    },
    set(obj, prop, value) {
      console.log("set", prop, value);
      return Reflect.set(obj, prop, value);
    },
  }
);

const effect = () => {
  console.log(`Count is ${state.count}`);
};

effect();
state.count = 5;
effect();
