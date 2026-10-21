// expect: boom:x
// `'k' in x`, where no constituent of x declares k, narrows x to
// `x & Record<'k', unknown>` (TS 4.9); functions have a name.
function getMessage(cause: object): string {
  if ('message' in cause) return String(cause.message);
  return "";
}
function isTagged(e: unknown): boolean {
  if (typeof e !== 'object' || e === null || !('digest' in e) || typeof e.digest !== 'string') return false;
  const d: string = e.digest;
  return d.length > 0;
}
function nameOf(c: new (...args: any[]) => Error): string { return c.name; }
const named = function x() {};
getMessage({ message: "boom" }) + ":" + (isTagged({ digest: "d" }) && nameOf(Error) === "Error" ? named.name : "no");
