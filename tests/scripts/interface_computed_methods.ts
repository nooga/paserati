// Test computed property method syntax in interfaces
const methodSym = Symbol("method");
const propSym = Symbol("prop");
const optionalSym = Symbol("optional");

interface ComputedInterface {
  // Regular computed property
  [propSym]: string;
  
  // Computed method with shorthand syntax
  [methodSym](x: string): void;
  
  // Optional computed method 
  [optionalSym]?(value: number): boolean;
  
  // Mixed with regular properties
  normalProp: number;
  normalMethod(y: string): string;
}

// Test that the interface compiles correctly
let obj: ComputedInterface;

// expect: undefined