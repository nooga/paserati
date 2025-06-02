// expect: {x: "xx"}

// Define interface for the object we want to create
interface KupaInstance {
  x: string;
}

// Define interface for a factory function that creates instances
interface KupaFactory {
  create: () => KupaInstance;
}

// Create a factory object that implements the interface
const kupaFactory: KupaFactory = {
  create: function () {
    return { x: "xx" };
  },
};

// Use the factory to create instances
const instance1 = kupaFactory.create();
const instance2 = kupaFactory.create();

// Both should have proper typing
let typed: KupaInstance = instance1;

typed;
