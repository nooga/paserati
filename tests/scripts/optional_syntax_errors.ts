// Test file for optional syntax error cases
// This file contains intentional type errors that should be caught by the type checker
// expect_compile_error: Type Error

// Object type with missing required field - should be an error
type RequiredFields = {
  required: string;
  optional?: string;
};

// This should be valid
let validObject: RequiredFields = {
  required: "test",
  optional: "also test",
};

// This should also be valid (optional field omitted)
let validMinimal: RequiredFields = {
  required: "test",
};

// This should cause a type error - missing required field
let invalidObject: RequiredFields = {
  optional: "test", // required field is missing
};

// Interface with missing required method - should be an error
interface ServiceInterface {
  requiredMethod(): void;
  optionalMethod?(): void;
}

// This should be valid
let validService: ServiceInterface = {
  requiredMethod(): void {
    // implementation
  },
  optionalMethod(): void {
    // optional method provided
  },
};

// This should also be valid (optional method omitted)
let validMinimalService: ServiceInterface = {
  requiredMethod(): void {
    // implementation
  },
};

// This should cause a type error - missing required method
let invalidService: ServiceInterface = {
  optionalMethod(): void {
    // only optional method provided, required missing
  },
};

// Object type with wrong optional field type - should be an error
type TypedOptional = {
  name: string;
  count?: number; // should be number if provided
};

// This should be valid
let validTyped: TypedOptional = {
  name: "test",
  count: 42,
};

// This should also be valid
let validTypedMinimal: TypedOptional = {
  name: "test",
};

// This should cause a type error - wrong type for optional field
let invalidTyped: TypedOptional = {
  name: "test",
  count: "not a number", // should be number, not string
};

// Interface with wrong optional method signature - should be an error
interface MethodInterface {
  process(input: string): string;
  validate?(input: string): boolean; // should return boolean if provided
}

// This should be valid
let validMethods: MethodInterface = {
  process(input: string): string {
    return "processed: " + input;
  },
  validate(input: string): boolean {
    return input.length > 0;
  },
};

// This should also be valid
let validMethodsMinimal: MethodInterface = {
  process(input: string): string {
    return "processed: " + input;
  },
};

// This should cause a type error - wrong return type for optional method
let invalidMethods: MethodInterface = {
  process(input: string): string {
    return "processed: " + input;
  },
  validate(input: string): string {
    // should return boolean, not string
    return "valid";
  },
};

// If we get here without type errors, something is wrong
("This should not execute due to type errors above");
