// 🚀 Paserati: TypeScript → Bytecode → Execution
// Zero transpilation, pure ES2025 + TypeScript

console.log("🚀 Paserati ES2025 Showcase\n");

// 1. Proxy-based Observable
console.log("🎭 Proxy-based Reactivity:");

const state = new Proxy(
  { count: 0, name: "init" },
  {
    set(obj, prop, value) {
      const old = obj[prop];
      obj[prop] = value;
      if (old !== value) {
        console.log(`  ⚡ ${String(prop)}: ${old} → ${value}`);
      }
      return true;
    }
  }
);

state.count = 42;
state.name = "reactive";
state.count = 100;

// 2. Generic Class with Type Constraints
console.log("\n🧬 Generic Repository:");

class DataStore<T> {
  private items: T[] = [];

  add(item: T): void {
    this.items.push(item);
  }

  all(): T[] {
    return this.items;
  }

  filter(predicate: (item: T) => boolean): T[] {
    return this.items.filter(predicate);
  }
}

const numbers = new DataStore<number>();
numbers.add(10);
numbers.add(25);
numbers.add(42);
numbers.add(18);

const large = numbers.filter(n => n > 20);
console.log(`  Stored ${numbers.all().length} numbers, ${large.length} are > 20:`);
console.log(`  → ${large.join(", ")}`);

// 3. Modern Destructuring & Spread
console.log("\n📦 Destructuring & Spread:");

const config = { host: "localhost", port: 8080, ssl: true, timeout: 5000 };
const { host, port, ...rest } = config;
const extended = { ...config, db: "postgres", cache: true };

console.log(`  Server: ${host}:${port}`);
console.log(`  Extended keys: ${Object.keys(extended).join(", ")}`);

// 4. Optional Chaining & Nullish Coalescing
console.log("\n🔗 Safe Navigation:");

const data1 = { user: { profile: { email: "alice@example.com" } } };
const data2 = { user: { profile: {} } };
const data3: any = {};

console.log(`  data1: ${data1.user?.profile?.email ?? "N/A"}`);
console.log(`  data2: ${data2.user?.profile?.email ?? "N/A"}`);
console.log(`  data3: ${data3.user?.profile?.email ?? "N/A"}`);

// 5. Generator Functions
console.log("\n🔄 Generators:");

function* fibonacci(n: number): Generator<number, void, undefined> {
  let [a, b] = [0, 1];
  for (let i = 0; i < n; i++) {
    yield a;
    [a, b] = [b, a + b];
  }
}

const fibs: number[] = [];
for (const num of fibonacci(10)) {
  fibs.push(num);
}
console.log(`  Fibonacci(10): ${fibs.join(", ")}`);

// 6. Async/Await with Promises
console.log("\n📡 Async Pipeline:");

async function fetchData(id: string) {
  return Promise.resolve({ id, timestamp: Date.now() });
}

async function pipeline() {
  const result1 = await fetchData("user-1");
  const result2 = await fetchData("user-2");
  const result3 = await fetchData("user-3");

  console.log(`  ✓ Fetched 3 items:`);
  console.log(`    → ${result1.id}`);
  console.log(`    → ${result2.id}`);
  console.log(`    → ${result3.id}`);
}

// 7. Async Generators
console.log("\n⚡ Async Generators:");

async function* countAsync(n: number): AsyncGenerator<number, void, undefined> {
  for (let i = 1; i <= n; i++) {
    await Promise.resolve(null);
    yield i;
  }
}

async function consumeGenerator() {
  let sum = 0;
  for await (const num of countAsync(5)) {
    sum += num;
    console.log(`  → Yielded: ${num}, sum: ${sum}`);
  }
}

// 8. Map & Set Collections
console.log("\n🗺️  Modern Collections:");

const users = new Map();
users.set("alice", { age: 28, role: "dev" });
users.set("bob", { age: 35, role: "lead" });

console.log(`  Map size: ${users.size}`);
console.log(`  alice: ${JSON.stringify(users.get("alice"))}`);

const tags = new Set(["typescript", "go", "rust", "typescript"]);
console.log(`  Set size: ${tags.size} (duplicates removed)`);

const tagArray: string[] = [];
for (const tag of tags) {
  tagArray.push(tag);
}
console.log(`  Tags: ${tagArray.join(", ")}`);

// Execute async functions
pipeline().then(() => {
  return consumeGenerator();
}).then(() => {
  console.log("\n✨ All ES2025 features working in Paserati!");
});
