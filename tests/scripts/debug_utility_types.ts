// expect: debug works

// Debug test to see if utility types are being recognized

type Person = { name: string; age: number };

// Test if Partial is recognized at all
type PartialPerson = Partial<Person>;

"debug works";