// expect: 1100

let obj = {
  functions: [
    { name: "add100", body: (x: number) => x + 100 },
    { name: "add200", body: (x: number) => x + 200 },
  ],
  final: () => ({
    name: "final",
    body: (x: number) => x + 300,
  }),
};

let result = 0;

for (let i = 0; i < obj.functions.length; i++) {
  result += obj.functions[i].body(result);
}

result += obj.final().body(result);

result;
