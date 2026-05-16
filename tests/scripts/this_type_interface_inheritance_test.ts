// expect: ok
interface ChainA {
  foo(): this;
}

interface ChainB extends ChainA {
  bar(): this;
}

interface ChainC extends ChainB {
  baz(): string;
}

let chain: ChainC = {
  foo() {
    return this;
  },
  bar() {
    return this;
  },
  baz() {
    return "ok";
  },
};

chain.foo().bar().baz();
