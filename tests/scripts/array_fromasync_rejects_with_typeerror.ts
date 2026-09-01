// expect: TypeError:true:true
// Array.fromAsync's argument-validation rejections must carry a real TypeError
// object, not its message flattened to a string (#147). The native side built a
// proper TypeError and then threw away everything but .message on the way into
// the rejected promise, so `e instanceof TypeError` was false and `e.message`
// was undefined in the handler.
let nullArg: any = null;
let notCallable: any = 5;

let first = "no-throw";
let firstIsTypeError = false;
try {
  await Array.fromAsync(nullArg);
} catch (e) {
  first = e.name;
  firstIsTypeError =
    e instanceof TypeError &&
    e.message === "Cannot convert undefined or null to object";
}

let secondIsTypeError = false;
try {
  await Array.fromAsync([1], notCallable);
} catch (e) {
  secondIsTypeError = e instanceof TypeError;
}

first + ":" + firstIsTypeError + ":" + secondIsTypeError;
