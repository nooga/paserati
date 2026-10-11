// expect: nodom
// `typeof globalThis` is the type of the global object, not any: without the
// DOM lib there is no onmessage, so @types/node's conditionals pick their
// false branch.
type Probe = typeof globalThis extends { onmessage: any } ? "dom" : "nodom";
const p: Probe = "nodom";
p;
