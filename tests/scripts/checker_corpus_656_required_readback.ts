// expect: application/json|1|any-ok
interface Opts { headers?: Record<string, string>; timeout?: number }
const o: Required<Opts> = { headers: {}, timeout: 1 };
if (!o.headers["Content-Type"]) o.headers["Content-Type"] = "application/json";
const t: number = o.timeout;

declare const rpc: any;
type C = ReturnType<typeof rpc.connect>;
let client: C | undefined;
[o.headers["Content-Type"], t, client === undefined ? "any-ok" : "no"].join("|");
