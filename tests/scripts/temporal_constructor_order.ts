// Temporal constructors convert and validate their arguments before they
// read newTarget.prototype (OrdinaryCreateFromConstructor comes last).
// no-typecheck
// expect: arg,proto|arg|RangeError
const log: string[] = [];
const arg = (v: number) => ({ valueOf() { log.push("arg"); return v; } });
// A bound function has no own "prototype", so a logging getter can be defined.
const nt: any = function () {}.bind(null);
Object.defineProperty(nt, "prototype", { get() { log.push("proto"); return Temporal.PlainDate.prototype; } });
Reflect.construct(Temporal.PlainDate, [arg(2024), 3, 10], nt);
const ok = log.join(",");
log.length = 0;
let err = "none";
try { Reflect.construct(Temporal.PlainDate, [arg(2024), 13, 10], nt); } catch (e) { err = (e as Error).constructor.name; }
// A month of 13 is rejected before the prototype is touched.
ok + "|" + log.join(",") + "|" + err;
