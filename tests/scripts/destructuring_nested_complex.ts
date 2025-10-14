// Test complex nested destructuring
// expect: 6:John:Main St:12345

let a = 0;
let b = 0;
let c = 0;
let name = "";
let street = "";
let zip = "";

[
  a,
  [b, c],
  {
    person: { name },
    address: { street, zip },
  },
] = [
  1,
  [2, 3],
  {
    person: { name: "John" },
    address: { street: "Main St", zip: "12345" },
  },
] as [
  number,
  [number, number],
  { person: { name: string }; address: { street: string; zip: string } }
];

let result = a + b + c;
let info = name + ":" + street + ":" + zip;
let final = result + ":" + info;
final;
