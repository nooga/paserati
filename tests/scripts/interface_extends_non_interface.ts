// expect_compile_error: 'StringType' is not an interface, cannot extend
// This should produce a type error - extending a type alias

type StringType = string;

interface Person extends StringType {
  name: string;
}

let person: Person = {
  name: "John",
};

person;
