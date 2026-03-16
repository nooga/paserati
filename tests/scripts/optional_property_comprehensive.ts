// expect: all optional property tests passed
// Comprehensive tests for optional properties with T | undefined semantics

// ==========================================
// 1. Interface optional properties
// ==========================================

type Level = "low" | "medium" | "high";

interface Config {
  level?: Level;
  name: string;
}

function applyConfig(cfg: Config): string {
  if (cfg.level !== undefined) {
    return cfg.level;
  }
  return "default";
}

const r1 = applyConfig({ name: "test", level: "high" });
const r2 = applyConfig({ name: "test" });
if (r1 !== "high" || r2 !== "default") throw "FAIL interface optional";

// ==========================================
// 2. Class optional properties
// ==========================================

class Agent {
  level?: Level;
  name: string;

  constructor(name: string) {
    this.name = name;
  }

  getLevel(): string {
    if (this.level !== undefined) {
      return this.level;
    }
    return "none";
  }
}

const agent1 = new Agent("a1");
agent1.level = "low";
const agent2 = new Agent("a2");
if (agent1.getLevel() !== "low" || agent2.getLevel() !== "none") throw "FAIL class optional";

// ==========================================
// 3. Loose equality (!=) with undefined
// ==========================================

function applyLoose(cfg: Config): string {
  if (cfg.level != undefined) {
    return cfg.level;
  }
  return "default";
}

if (applyLoose({ name: "t", level: "medium" }) !== "medium") throw "FAIL loose equality";

// ==========================================
// 4. Non-string literal union optional
// ==========================================

interface Timing {
  delayMs?: 100 | 500 | 1000;
}

function getDelay(t: Timing): number {
  if (t.delayMs !== undefined) {
    return t.delayMs;
  }
  return 0;
}

if (getDelay({ delayMs: 500 }) !== 500 || getDelay({}) !== 0) throw "FAIL numeric union optional";

// ==========================================
// 5. Optional scalar types
// ==========================================

interface OptionalScalars {
  count?: number;
  label?: string;
  active?: boolean;
}

function describeScalars(o: OptionalScalars): string {
  let parts: string[] = [];
  if (o.count !== undefined) {
    parts.push("count:" + o.count);
  }
  if (o.label !== undefined) {
    parts.push("label:" + o.label);
  }
  if (o.active !== undefined) {
    parts.push("active:" + o.active);
  }
  return parts.join(",");
}

if (describeScalars({ count: 5, label: "x" }) !== "count:5,label:x") throw "FAIL scalar optional";
if (describeScalars({}) !== "") throw "FAIL scalar optional empty";

// ==========================================
// 6. Optional interface-typed property
// ==========================================

interface Address {
  street: string;
  city: string;
}

interface Person {
  name: string;
  address?: Address;
}

function getCity(p: Person): string {
  if (p.address !== undefined) {
    return p.address.city;
  }
  return "unknown";
}

if (getCity({ name: "Alice", address: { street: "1st", city: "NYC" } }) !== "NYC") throw "FAIL optional interface";
if (getCity({ name: "Bob" }) !== "unknown") throw "FAIL optional interface missing";

// ==========================================
// 7. Optional with generics
// ==========================================

interface Box<T> {
  value?: T;
}

function unbox<T>(b: Box<T>, fallback: T): T {
  if (b.value !== undefined) {
    return b.value;
  }
  return fallback;
}

if (unbox({ value: 42 }, 0) !== 42) throw "FAIL generic optional with value";
if (unbox({}, 99) !== 99) throw "FAIL generic optional without value";

// ==========================================
// 8. Optional function-typed property
// ==========================================

interface Handler {
  onSuccess?: (result: string) => string;
  onError?: (error: string) => string;
}

function runHandler(h: Handler, ok: boolean): string {
  if (ok && h.onSuccess !== undefined) {
    return h.onSuccess("done");
  }
  if (!ok && h.onError !== undefined) {
    return h.onError("failed");
  }
  return "no handler";
}

const h: Handler = {
  onSuccess: (r: string) => "OK:" + r
};
if (runHandler(h, true) !== "OK:done") throw "FAIL optional function";
if (runHandler(h, false) !== "no handler") throw "FAIL optional function missing";

// ==========================================
// 9. Truthiness narrowing on optional members
// ==========================================

class LinkedNode<T> {
  value: T;
  next?: LinkedNode<T>;

  constructor(value: T) {
    this.value = value;
  }

  last(): T {
    if (this.next) {
      return this.next.last();
    }
    return this.value;
  }
}

const n1 = new LinkedNode(1);
const n2 = new LinkedNode(2);
n1.next = n2;
if (n1.last() !== 2) throw "FAIL truthiness narrowing";

// ==========================================
// 10. || short-circuit narrowing with optional
// ==========================================

interface Container {
  items?: string[];
}

function firstItem(c: Container): string {
  if (!c.items || c.items.length === 0) {
    return "empty";
  }
  return c.items[0];
}

if (firstItem({ items: ["a", "b"] }) !== "a") throw "FAIL || narrowing";
if (firstItem({}) !== "empty") throw "FAIL || narrowing empty";

// ==========================================
// All tests passed
// ==========================================
"all optional property tests passed";
