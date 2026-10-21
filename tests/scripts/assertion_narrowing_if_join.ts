// expect: hi|undefined
// An assertion call inside an if arm narrows the rest of the arm, so the
// join after the if sees the asserted type (tRPC's parseTRPCMessage).
function assertIsString(v: unknown): asserts v is string {
  if (typeof v !== "string") throw new Error("not a string");
}
function pick(v: unknown): string | undefined {
  if (v !== undefined) {
    assertIsString(v);
  }
  const r: string | undefined = v;
  return r;
}
pick("hi") + "|" + pick(undefined);
