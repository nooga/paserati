// expect: annotated destructuring uses declared types

var [a, b]: [number, any] = [undefined, undefined];
let [x]: [string | number] = [1];
let [y]: [string | undefined] = [""];
let [z = ""]: [string | undefined] = [undefined];
let { q }: { q?: string | number } = {};
let { r = "" }: { r?: string | undefined } = {};

"annotated destructuring uses declared types";
