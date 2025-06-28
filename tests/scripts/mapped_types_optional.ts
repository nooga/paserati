// expect: optional mapping works

// Test mapped types with optional modifier

type Person = { name: string; age: number };

// Make all properties optional
type PartialPerson = { [P in keyof Person]?: Person[P] };

"optional mapping works";