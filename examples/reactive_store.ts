// Reactive Store Example - Showcasing Paserati's Type System
// Demonstrates: Generics, Proxy, Type Constraints, Async/Await

// Helper: Type-preserving reactive wrapper using Proxy
function reactive<T extends object>(target: T): T {
  return new Proxy(target, {
    set(obj, prop, value) {
      const old = obj[prop as keyof T];
      obj[prop as keyof T] = value;
      if (old !== value) {
        console.log(`  🔄 ${String(prop)}: ${old} → ${value}`);
      }
      return true;
    }
  });
}

// Generic reactive store with type constraints and async methods
class ReactiveStore<T extends { id: string }> {
  private items: T[] = [];

  add(item: T): T {
    const reactiveItem = reactive(item);
    this.items.push(reactiveItem);
    console.log(`  ✓ Added: ${JSON.stringify(item)}`);
    return reactiveItem;
  }

  filter(predicate: (item: T) => boolean): T[] {
    return this.items.filter(predicate);
  }

  async processAll(fn: (item: T) => Promise<void>): Promise<void> {
    for (const item of this.items) {
      await fn(item);
    }
  }

  count(): number {
    return this.items.length;
  }
}

// Type-safe usage
interface User {
  id: string;
  name: string;
  score: number;
}

console.log("🚀 Reactive Store Demo\n");

console.log("Adding users:");
const store = new ReactiveStore<User>();
const alice = store.add({ id: "1", name: "Alice", score: 100 });
const bob = store.add({ id: "2", name: "Bob", score: 85 });
const charlie = store.add({ id: "3", name: "Charlie", score: 120 });

console.log("\nReactive updates:");
alice.score = 150;
bob.score = 95;
charlie.name = "Chuck";

console.log("\nFiltering:");
const topUsers = store.filter(u => u.score > 90);
console.log(`  Found ${topUsers.length}/${store.count()} users with score > 90`);

console.log("\nAsync processing:");
await store.processAll(async (user) => {
  await Promise.resolve(null);  // Simulate async work
  console.log(`  → Processed ${user.name} (${user.score} pts)`);
});

console.log("\n✨ Demo complete!");
