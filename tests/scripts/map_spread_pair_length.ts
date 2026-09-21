// expect: 2
const m = new Map([["a", 1]]);
const spread = [...m];
spread[0].length;
