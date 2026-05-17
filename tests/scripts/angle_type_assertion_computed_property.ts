// expect: 42
let obj = { [<any>true]: 42 };
obj["true"];
