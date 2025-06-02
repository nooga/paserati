// expect: {name: "John", score: 95}

interface Named {
  name: string;
}

interface Student extends Named {
  name: string; // This overrides the inherited name property
  score: number;
}

let student: Student = {
  name: "John",
  score: 95,
};

student;
