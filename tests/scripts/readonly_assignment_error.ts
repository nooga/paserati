// Test readonly assignment error
class Person {
  readonly id = 1;
  name = "Alice";

  tryToModifyReadonly() {
    this.id = 2; // Should cause compile error
  }

  modifyNormal() {
    this.name = "Bob"; // Should work
  }
}

let person = new Person();
person.id = 3; // Should cause compile error

// expect_compile_error: cannot assign to readonly property
