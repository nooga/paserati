// expect_compile_error: unknown type name: NonExistentInterface
// This should produce a type error

interface Person extends NonExistentInterface {
  name: string;
}

let person: Person = {
  name: "John",
};

person;
