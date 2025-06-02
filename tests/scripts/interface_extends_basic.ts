// expect: {name: "John", age: 30, id: 1}

interface Named {
  name: string;
}

interface Aged {
  age: number;
}

interface Person extends Named, Aged {
  id: number;
}

let person: Person = {
  name: "John",
  age: 30,
  id: 1,
};

person;
