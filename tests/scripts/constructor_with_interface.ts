// expect: {}

// Define interface for the object we want to create
interface KupaInstance {
  x: string;
}

// Define interface for constructor function with property-style function type
interface KupaConstructor {
  construct: () => KupaInstance;
}

// Create constructor function with proper typing
const Kupa: KupaConstructor = {
  construct: function () {
    let value: string = "xx";
    return { x: value };
  },
};

// Use the construct method
const instance = Kupa.construct();

instance;
