// Complex example combining generics, mapped types, and tuples
// expect: Alice:30

// A type-safe record builder with tuple values

type Schema<K extends string> = { [P in K]: [string, number] };

function createRecord<K extends string>(
  keys: K[],
  values: { [P in K]: [string, number] }
): { [P in K]: [string, number] } {
  return values;
}

// Build a record with explicit schema
const people = createRecord<"alice" | "bob">(
  ["alice", "bob"],
  {
    alice: ["Alice", 30],
    bob: ["Bob", 25]
  }
);

// Access tuple elements
const aliceName: string = people.alice[0];
const aliceAge: number = people.alice[1];

// Combine result
aliceName + ":" + aliceAge;
