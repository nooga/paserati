// expect: simple test

// Test simplest mapped type syntax parsing

// Mapped type with primitive constraint
type StringToNumber = { [P in string]: number };

"simple test";