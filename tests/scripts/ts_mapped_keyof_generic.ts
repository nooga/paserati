// expect: mapped keyof generic works

type Box<T> = { [P in keyof T]: { value: T[P] } };

interface Person {
  name: string;
  age: number;
}

let boxed: Box<Person> = {
  name: { value: "Ada" },
  age: { value: 37 },
};

let nameValue: string = boxed.name.value;
let ageValue: number = boxed.age.value;

"mapped keyof generic works";
