// expect: boom|42|destr7
// TypeScript allows a type annotation on the catch binding, but only 'any' or
// 'unknown' (anything else is TS1196). The parser used to reject the colon with
// "')' expected".
function f(): string {
  try {
    throw new Error("boom");
  } catch (e: unknown) {
    if (e instanceof Error) return e.message;
    return String(e);
  }
}
function g(): number {
  try {
    throw 41;
  } catch (e: any) {
    return e + 1;
  }
}
function h(): string {
  try {
    throw { message: "destr", code: 7 };
  } catch ({ message, code }: any) {
    return message + code;
  }
}
[f(), g(), h()].join("|");
