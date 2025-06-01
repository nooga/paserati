// expect_compile_error: extended interface 'NonExistentInterface' is not defined
// This should produce a type error

interface Person extends NonExistentInterface {
  name: string;
}

let person: Person = {
  name: "John",
};

person;
