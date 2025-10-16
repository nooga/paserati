// 🚀 Paserati ES2025 Showcase
// Demonstrating async/await, Proxy, generators, and advanced TypeScript

console.log("🚀 Paserati ES2025 Feature Showcase\n");

// 1. Async/Await with Real Promises
console.log("📡 Async Data Pipeline:");
async function fetchData<T>(id: string, delay: number = 10): Promise<T> {
  return new Promise((resolve) => {
    // Simulate async operation without setTimeout (we don't have it yet)
    resolve({ id, fetched: true, timestamp: Date.now() } as T);
  });
}

interface DataItem {
  id: string;
  fetched: boolean;
  timestamp: number;
}

async function processDataPipeline() {
  const results = await Promise.all([
    fetchData<DataItem>("user-123", 5),
    fetchData<DataItem>("post-456", 8),
    fetchData<DataItem>("comment-789", 3),
  ]);

  console.log(`✓ Fetched ${results.length} items asynchronously`);
  results.forEach(r => console.log(`  → ${r.id} @ ${r.timestamp}`));
}

// 2. Proxy-based Observable Pattern
console.log("\n🎭 Proxy-based Reactivity:");
interface State {
  count: number;
  name: string;
  items: string[];
}

function createObservable<T extends object>(target: T, onChange: (key: keyof T, value: any) => void): T {
  return new Proxy(target, {
    set(obj, prop, value) {
      const oldValue = obj[prop as keyof T];
      obj[prop as keyof T] = value;
      if (oldValue !== value) {
        onChange(prop as keyof T, value);
      }
      return true;
    }
  });
}

const state = createObservable<State>(
  { count: 0, name: "initial", items: [] },
  (key, val) => console.log(`  ⚡ ${String(key)} changed to: ${JSON.stringify(val)}`)
);

state.count = 42;
state.name = "reactive";
state.items = ["a", "b", "c"];

// 3. Async Generator with Type Safety
console.log("\n🔄 Async Generator Pipeline:");
async function* generateSequence<T>(items: T[]): AsyncGenerator<T, void, undefined> {
  for (const item of items) {
    await Promise.resolve(null); // Yield control to event loop
    yield item;
  }
}

async function consumeAsyncGenerator() {
  const data = ["TypeScript", "Generics", "Async", "Generators"];
  let count = 0;

  for await (const item of generateSequence(data)) {
    console.log(`  → Item ${++count}: ${item}`);
  }
}

// 4. Advanced Generics with Constraints
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

  count(): number {
    return this.items.size;
  }
}

interface User {
  id: string;
  name: string;
  age: number;
}

const userRepo = new Repository<User>();
userRepo.add({ id: "1", name: "Alice", age: 28 });
userRepo.add({ id: "2", name: "Bob", age: 35 });
userRepo.add({ id: "3", name: "Charlie", age: 22 });

const adults = userRepo.filter(u => u.age >= 25);
console.log(`  Stored ${userRepo.count()} users, found ${adults.length} adults:`);
adults.forEach(u => console.log(`    • ${u.name} (${u.age})`));

// 5. Destructuring with Rest/Spread
console.log("\n📦 Destructuring & Spread:");
const config = { host: "localhost", port: 8080, ssl: true, timeout: 5000 };
const { host, port, ...rest } = config;
const extended = { ...config, database: "postgres", cache: true };

console.log(`  Server: ${host}:${port}`);
console.log(`  Other options: ${JSON.stringify(rest)}`);
console.log(`  Extended config has ${Object.keys(extended).length} keys`);

// 6. Optional Chaining & Nullish Coalescing
console.log("\n🔗 Modern Operators:");
interface NestedData {
  user?: {
    profile?: {
      email?: string;
    };
  };
}

const data1: NestedData = { user: { profile: { email: "test@example.com" } } };
const data2: NestedData = { user: {} };
const data3: NestedData = {};

console.log(`  data1.email: ${data1.user?.profile?.email ?? "N/A"}`);
console.log(`  data2.email: ${data2.user?.profile?.email ?? "N/A"}`);
console.log(`  data3.email: ${data3.user?.profile?.email ?? "N/A"}`);

// Main execution
(async () => {
  await processDataPipeline();
  await consumeAsyncGenerator();
  console.log("\n✨ All features working perfectly in Paserati!");
})();
