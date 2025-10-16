// 🚀 Paserati ES2025 Showcase
// TypeScript → Bytecode → Execution, zero transpilation

console.log("🚀 Paserati ES2025 Feature Showcase\n");

// 1. Proxy-based Observable Pattern
console.log("🎭 Proxy-based Reactivity:");
function createObservable<T extends object>(target: T, onChange: (key: string, val: any) => void): T {
  return new Proxy(target, {
    set(obj, prop, value) {
      const old = obj[prop as keyof T];
      obj[prop as keyof T] = value;
      if (old !== value) onChange(String(prop), value);
      return true;
    }
  });
}

interface State {
  count: number;
  items: string[];
}

const state = createObservable<State>(
  { count: 0, items: [] },
  (key, val) => console.log(`  ⚡ ${key} → ${JSON.stringify(val)}`)
);

state.count = 42;
state.items = ["TypeScript", "Bytecode", "VM"];

// 2. Generic Repository with Type Constraints
console.log("\n🧬 Generic Type System:");
class Repository<T extends { id: string }> {
  private items = new Map<string, T>();

  add(item: T): void {
    this.items.set(item.id, item);
  }

  find(id: string): T | undefined {
    return this.items.get(id);
  }

  filter(predicate: (item: T) => boolean): T[] {
    return Array.from(this.items.values()).filter(predicate);
  }
}

interface User {
  id: string;
  name: string;
  age: number;
  skills: string[];
}

const users = new Repository<User>();
users.add({ id: "1", name: "Alice", age: 28, skills: ["TS", "Go"] });
users.add({ id: "2", name: "Bob", age: 35, skills: ["Rust", "C++"] });
users.add({ id: "3", name: "Charlie", age: 22, skills: ["JS", "Python"] });

const adults = users.filter(u => u.age >= 25);
console.log(`  Found ${adults.length} adult users:`);
adults.forEach(u => console.log(`    • ${u.name}, ${u.age}, [${u.skills.join(", ")}]`));

// 3. Destructuring & Modern Operators
console.log("\n📦 Destructuring & Spread:");
const config = { host: "localhost", port: 8080, ssl: true, timeout: 5000 };
const { host, port, ...security } = config;
const extended = { ...config, database: "postgres", cache: true };

console.log(`  Server: ${host}:${port}`);
console.log(`  Security: ${JSON.stringify(security)}`);
console.log(`  Extended config has ${Object.keys(extended).length} keys`);

// 4. Optional Chaining & Nullish Coalescing
console.log("\n🔗 Safe Navigation:");
interface ApiResponse {
  data?: {
    user?: {
      profile?: {
        email?: string;
      };
    };
  };
}

const responses: ApiResponse[] = [
  { data: { user: { profile: { email: "alice@example.com" } } } },
  { data: { user: {} } },
  { data: {} },
  {}
];

for (let i = 0; i < responses.length; i++) {
  const email = responses[i].data?.user?.profile?.email ?? "N/A";
  console.log(`  Response ${i + 1}: ${email}`);
}

// 5. Generator Functions
console.log("\n🔄 Generator Pipeline:");
function* fibonacci(n: number): Generator<number, void, undefined> {
  let [a, b] = [0, 1];
  for (let i = 0; i < n; i++) {
    yield a;
    [a, b] = [b, a + b];
  }
}

const fibs = Array.from(fibonacci(10));
console.log(`  First 10 Fibonacci: ${fibs.join(", ")}`);

// 6. Async/Await with Promises
console.log("\n📡 Async Data Pipeline:");

interface DataItem {
  id: string;
  timestamp: number;
}

async function processData() {
  const p1 = Promise.resolve({ id: "user-1", timestamp: Date.now() });
  const p2 = Promise.resolve({ id: "user-2", timestamp: Date.now() });
  const p3 = Promise.resolve({ id: "user-3", timestamp: Date.now() });

  const results = await Promise.all([p1, p2, p3]);

  console.log(`  ✓ Fetched ${results.length} items asynchronously`);
  console.log(`    → ${results[0].id} @ ${results[0].timestamp}`);
  console.log(`    → ${results[1].id} @ ${results[1].timestamp}`);
  console.log(`    → ${results[2].id} @ ${results[2].timestamp}`);
}

// 7. Async Generators
console.log("\n⚡ Async Generator:");
async function* generateAsync<T>(items: T[]): AsyncGenerator<T, void, undefined> {
  for (const item of items) {
    await Promise.resolve(null);
    yield item;
  }
}

async function consumeAsync() {
  const data = ["Async", "Generators", "Working"];
  let count = 0;
  for await (const item of generateAsync(data)) {
    console.log(`  → Item ${++count}: ${item}`);
  }
}

// Execute async operations
processData().then(() => {
  return consumeAsync();
}).then(() => {
  console.log("\n✨ All ES2025 features working in Paserati!");
});
