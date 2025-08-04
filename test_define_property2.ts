// Test Object.defineProperty and getOwnPropertyDescriptor
let obj: any = {};

// Test defineProperty
Object.defineProperty(obj, "foo", { value: 42 });
console.log("obj.foo:", obj.foo);

// Test getOwnPropertyDescriptor
let desc = Object.getOwnPropertyDescriptor(obj, "foo");
console.log("descriptor:", desc);
console.log("descriptor.value:", desc?.value);
console.log("descriptor.writable:", desc?.writable);