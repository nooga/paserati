// expect: all optional chaining narrowing tests passed
// Tests for optional chaining on optional parameters with narrowing

// ==========================================
// 1. Property assignment after guard (repro 1)
// ==========================================

interface Opts {
  temperature?: number;
  label?: string;
}

interface Result {
  temperature?: number;
  label?: string;
}

function apply(opts?: Opts): Result {
  const r: Result = {};
  if (opts?.temperature !== undefined) {
    r.temperature = opts.temperature;
  }
  if (opts?.label !== undefined) {
    r.label = opts.label;
  }
  return r;
}

const r1 = apply({ temperature: 42, label: "test" });
if (r1.temperature !== 42 || r1.label !== "test") throw "FAIL repro1 with values";
const r2 = apply();
if (r2.temperature !== undefined || r2.label !== undefined) throw "FAIL repro1 without values";

// ==========================================
// 2. Optional chaining with object-typed property (repro 3)
// ==========================================

interface Config {
  schema?: object;
  verbs?: string[];
}

interface OutputCfg {
  schema?: object;
}

function makeOutput(cfg?: Config): OutputCfg {
  const out: OutputCfg = {};
  if (cfg?.schema !== undefined) {
    out.schema = cfg.schema;
  }
  return out;
}

const o1 = makeOutput({ schema: { type: "test" } });
const o2 = makeOutput();

// ==========================================
// 3. Multiple optional chaining guards in sequence
// ==========================================

interface FullOpts {
  verbose?: boolean;
  retries?: number;
  name?: string;
}

interface FullResult {
  verbose?: boolean;
  retries?: number;
  name?: string;
}

function applyFull(opts?: FullOpts): FullResult {
  const r: FullResult = {};
  if (opts?.verbose !== undefined) {
    r.verbose = opts.verbose;
  }
  if (opts?.retries !== undefined) {
    r.retries = opts.retries;
  }
  if (opts?.name !== undefined) {
    r.name = opts.name;
  }
  return r;
}

const f1 = applyFull({ verbose: true, retries: 5, name: "test" });
if (f1.verbose !== true || f1.retries !== 5 || f1.name !== "test") throw "FAIL full opts";
const f2 = applyFull();
if (f2.verbose !== undefined) throw "FAIL full opts empty";

// ==========================================
// 4. Optional chaining with return value narrowing
// ==========================================

function getRetries(s?: FullOpts): number {
  if (s?.retries !== undefined) {
    return s.retries;
  }
  return 3;
}

if (getRetries({ retries: 5 }) !== 5) throw "FAIL retries with value";
if (getRetries() !== 3) throw "FAIL retries default";

function getName(s?: FullOpts): string {
  if (s?.name !== undefined) {
    return s.name;
  }
  return "anonymous";
}

if (getName({ name: "Alice" }) !== "Alice") throw "FAIL name with value";
if (getName() !== "anonymous") throw "FAIL name default";

// ==========================================
// 5. Optional chaining with loose equality
// ==========================================

function getNameLoose(s?: FullOpts): string {
  if (s?.name != undefined) {
    return s.name;
  }
  return "anon";
}

if (getNameLoose({ name: "Bob" }) !== "Bob") throw "FAIL loose equality";
if (getNameLoose() !== "anon") throw "FAIL loose equality default";

// ==========================================
// 6. Nested interface with optional chaining
// ==========================================

interface Inner {
  value: number;
}

interface Outer {
  inner?: Inner;
}

function getInnerValue(o?: Outer): number {
  if (o?.inner !== undefined) {
    return o.inner.value;
  }
  return -1;
}

if (getInnerValue({ inner: { value: 99 } }) !== 99) throw "FAIL nested";
if (getInnerValue() !== -1) throw "FAIL nested default";
if (getInnerValue({}) !== -1) throw "FAIL nested missing inner";

// ==========================================
// 7. Optional chaining guard on class method parameter
// ==========================================

class Processor {
  process(opts?: Opts): number {
    if (opts?.temperature !== undefined) {
      return opts.temperature;
    }
    return 0;
  }
}

const proc = new Processor();
if (proc.process({ temperature: 100 }) !== 100) throw "FAIL class method";
if (proc.process() !== 0) throw "FAIL class method default";

// ==========================================
// 8. Optional chaining with union type property
// ==========================================

type Priority = "low" | "medium" | "high";

interface Task {
  priority?: Priority;
}

interface TaskResult {
  priority?: Priority;
}

function getTask(t?: Task): TaskResult {
  const r: TaskResult = {};
  if (t?.priority !== undefined) {
    r.priority = t.priority;
  }
  return r;
}

const t1 = getTask({ priority: "high" });
if (t1.priority !== "high") throw "FAIL union type";
const t2 = getTask();
if (t2.priority !== undefined) throw "FAIL union type default";

"all optional chaining narrowing tests passed";
