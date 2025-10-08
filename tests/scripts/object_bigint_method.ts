// expect: bar
let o = { 1n() { return "bar"; } };
o["1"]();
