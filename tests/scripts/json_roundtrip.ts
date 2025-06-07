// Test JSON roundtrip: stringify then parse
let original = {
  name: "Charlie",
  age: 25,
  hobbies: ["reading", "coding"],
  active: true,
  metadata: {
    created: 2023,
    tags: ["user", "developer"],
  },
};

let jsonString = JSON.stringify(original);
let parsed = JSON.parse(jsonString);

// Check that roundtrip preserves data
let roundtripCorrect =
  parsed.name === original.name &&
  parsed.age === original.age &&
  parsed.hobbies[0] === original.hobbies[0] &&
  parsed.hobbies[1] === original.hobbies[1] &&
  parsed.active === original.active &&
  parsed.metadata.created === original.metadata.created &&
  parsed.metadata.tags[0] === original.metadata.tags[0];

// expect: true
roundtripCorrect;
