// expect_compile_error: Cannot find name 'NonExistentInterface'.
// This should produce a type error

interface Person extends NonExistentInterface {
  name: string;
}

let person: Person = {
  name: "John",
};

person;
