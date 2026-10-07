// expect: {"0":"a","1":"b"}|{"0":"a","1":"b"}|{"0":"c"}|d83d
// #607: CopyDataProperties does ToObject on a string source
const s: any = "ab";
const spread = JSON.stringify({ ...s });
const assigned = JSON.stringify(Object.assign({}, "ab"));
const mixed = JSON.stringify(Object.assign({}, null, 1, true, "c"));
const emoji: any = "\ud83d\ude00";
const pair: any = { ...emoji };
[spread, assigned, mixed, pair[0].charCodeAt(0).toString(16)].join("|");
