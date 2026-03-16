// expect: all tests passed
// Tests for optional chaining narrowing where the base is a member expression
// e.g., req.options?.effort where options is an optional property

interface Opts {
  temperature?: number;
  effort?: string;
}

interface Request {
  options?: Opts;
}

// 1. Basic: req.options?.effort guard narrows req.options
function getEffort(req: Request): string {
  if (req.options?.effort !== undefined) {
    return req.options.effort;
  }
  return "default";
}

if (getEffort({ options: { effort: "high" } }) !== "high") throw "FAIL 1a";
if (getEffort({}) !== "default") throw "FAIL 1b";

// 2. Multiple guards on same base
function apply(req: Request): string {
  if (req.options?.effort !== undefined) {
    return "effort:" + req.options.effort;
  }
  if (req.options?.temperature !== undefined) {
    const t: number = req.options.temperature;
    return "temp:" + t;
  }
  return "none";
}

if (apply({ options: { effort: "low" } }) !== "effort:low") throw "FAIL 2a";
if (apply({ options: { temperature: 42 } }) !== "temp:42") throw "FAIL 2b";
if (apply({}) !== "none") throw "FAIL 2c";

// 3. Deeper nesting: obj.a?.b where a is optional
interface Inner {
  value: number;
}

interface Middle {
  inner?: Inner;
}

interface Outer {
  middle?: Middle;
}

function getInnerValue(o: Outer): number {
  if (o.middle?.inner !== undefined) {
    return o.middle.inner.value;
  }
  return -1;
}

if (getInnerValue({ middle: { inner: { value: 99 } } }) !== 99) throw "FAIL 3a";
if (getInnerValue({ middle: {} }) !== -1) throw "FAIL 3b";
if (getInnerValue({}) !== -1) throw "FAIL 3c";

// 4. Class property with optional chaining
class Service {
  config?: Opts;

  getEffort(): string {
    if (this.config?.effort !== undefined) {
      return this.config.effort;
    }
    return "default";
  }
}

const svc = new Service();
if (svc.getEffort() !== "default") throw "FAIL 4a";
svc.config = { effort: "medium" };
if (svc.getEffort() !== "medium") throw "FAIL 4b";

"all tests passed";
