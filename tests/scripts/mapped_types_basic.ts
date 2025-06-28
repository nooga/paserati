// expect: mapped types work

// Test basic mapped type functionality

// Define a base type
type Person = { name: string; age: number };

// Basic mapped type - all properties become string
type StringifiedPerson = { [P in keyof Person]: string };

// Test with optional modifier  
type OptionalPerson = { [P in keyof Person]?: Person };

// Test successful completion
"mapped types work";