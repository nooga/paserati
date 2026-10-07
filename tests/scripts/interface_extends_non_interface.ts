// expect_compile_error: An interface can only extend an object type or intersection of object types with statically known members.
// This should produce a type error - extending a type alias

type StringType = string;

interface Person extends StringType {
  name: string;
}

let person: Person = {
  name: "John",
};

person;
