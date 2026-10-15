// expect: 3
// A literal key T lacks reads T's string index signature.
interface Custom { a: string; [key: string]: any }
type X = Custom['anything'];
const x: X = 3;
x;
