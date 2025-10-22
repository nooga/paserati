// expect: E-E-G-G
// Test numeric property key access
// Per ECMAScript: obj[Infinity] and obj["Infinity"] should access the same property

let obj: any = {
    [Infinity]: "E",
    [NaN]: "G"
};

obj[Infinity] + "-" + obj["Infinity"] + "-" + obj[NaN] + "-" + obj["NaN"]
