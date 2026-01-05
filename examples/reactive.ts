// Reactive Store Example - Showcasing Paserati's Type System
// Demonstrates: Proxy, Classes, Async/Await

interface User {
  id: string;
  name: string;
  score: number;
}

// Type-preserving reactive wrapper using Proxy
function reactive(target: User): User {
  return new Proxy(target, {
    set(obj, prop, value) {
      const old = (obj as any)[prop];
      (obj as any)[prop] = value;
      if (old !== value) {
        console.log(`  Changed ${String(prop)}: ${old} -> ${value}`);
      }
      return true;
    }
  });
}

// Reactive store for users
class UserStore {
  private items: User[] = [];

  add(item: User): User {
    const reactiveItem = reactive(item);
    this.items.push(reactiveItem);
    console.log(`  Added: ${item.name} (id: ${item.id})`);
    return reactiveItem;
  }

  getAll(): User[] {
    return this.items;
  }

  async processAll(fn: (item: User) => Promise<void>): Promise<void> {
    for (const item of this.items) {
      await fn(item);
    }
  }

  count(): number {
    return this.items.length;
  }
}

console.log("=== Reactive Store Demo ===\n");

console.log("Adding users:");
const store = new UserStore();
const alice = store.add({ id: "1", name: "Alice", score: 100 });
const bob = store.add({ id: "2", name: "Bob", score: 85 });
const charlie = store.add({ id: "3", name: "Charlie", score: 120 });

console.log("\nReactive updates (changes trigger logging):");
alice.score = 150;
bob.score = 95;
charlie.name = "Chuck";

console.log("\nStore contains", store.count(), "users");

console.log("\nAsync processing:");
await store.processAll(async (user) => {
  await Promise.resolve(null);
  console.log(`  Processed ${user.name} (${user.score} pts)`);
});

console.log("\nDone!");
