// expect: 63

let obj = {
  a: 10,
  "b-prop": 20,
  c: 30,
};

let x = obj.a; // 10
let y = obj["b-prop"]; // 20

obj.a = x + y + 2; // obj.a = 10 + 20 + 2 = 32
obj.c += 1; // obj.c = 30 + 1 = 31 (Test compound assign involving member access)

// obj.newProp = obj.a + 1; // Assigning new prop

let result = obj.a + obj.c; // 32 + 31 = 63

result; // Final value to check
