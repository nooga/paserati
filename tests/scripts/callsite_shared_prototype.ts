// expect: false|true|0|CallSite|function function|true
// no-typecheck
let frame: any;
(Error as any).prepareStackTrace = (e: any, stack: any) => { frame = stack[0]; return "x"; };
new Error("e").stack;
(Error as any).prepareStackTrace = undefined;
const proto = Object.getPrototypeOf(frame);
const clone: any = {};
Object.getOwnPropertyNames(proto).forEach((n) => { clone[n] = frame[n]; });
[
  proto === Object.prototype,
  Object.getOwnPropertyNames(proto).includes("getFileName"),
  Object.getOwnPropertyNames(frame).length,
  frame.constructor.name,
  typeof clone.getFunctionName + " " + typeof clone.isNative,
  typeof clone.getLineNumber.call(frame) === "number",
].join("|");
