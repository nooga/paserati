// expect: {x: xx}

// Define interface for the object we want to create
interface ObjInstance {
  x: string;
}

// Define interface for constructor function with property-style function type
interface ObjConstructor {
  construct: () => ObjInstance;
}

// Create constructor function with proper typing
const Obj: ObjConstructor = {
  construct: function () {
    let value: string = "xx";
    return { x: value };
  },
};

// Use the construct method
const instance = Obj.construct();

instance;
