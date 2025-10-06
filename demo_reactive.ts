// Reactive State Management Demo for Paserati
// Demonstrates: Proxies, Reflect, Classes, Generics, Maps, Sets, Closures

type Effect = () => void;
type ComputedGetter<T> = () => T;

class ReactiveSystem {
  private currentEffect: Effect;
  private dependencies = new Map<object, Map<string, Set<Effect>>>();
  private computedCache = new Map<ComputedGetter<any>, any>();
  private computedDirty = new Map<ComputedGetter<any>, boolean>();
  private computedDeps = new Map<ComputedGetter<any>, Set<Effect>>();

  constructor() {
    this.currentEffect = () => {}; // dummy
  }

  // Create a reactive proxy that tracks property access
  reactive<T extends object>(target: T): T {
    const self = this;
    // Use target as the key for dependencies (consistent across get/set traps)
    const proxyTarget = target;

    return new Proxy(target, {
      get(obj, prop) {
        // Track this property access using the target as the key
        self.track(proxyTarget, prop.toString());
        return Reflect.get(obj, prop);
      },

      set(obj, prop, value) {
        const oldValue = Reflect.get(obj, prop);
        const result = Reflect.set(obj, prop, value);

        // Only trigger if value actually changed
        if (oldValue !== value) {
          self.trigger(proxyTarget, prop.toString());
        }

        return result;
      },
    });
  }

  // Track the current effect as dependent on this property
  private track(target: object, key: string) {
    let depsMap = this.dependencies.get(target);
    if (!depsMap) {
      depsMap = new Map();
      this.dependencies.set(target, depsMap);
    }

    let deps = depsMap.get(key);
    if (!deps) {
      deps = new Set();
      depsMap.set(key, deps);
    }

    deps.add(this.currentEffect);
  }

  // Trigger all effects that depend on this property
  private trigger(target: object, key: string) {
    const depsMap = this.dependencies.get(target);
    if (!depsMap) return;

    const deps = depsMap.get(key);
    if (!deps) return;

    // Run all dependent effects
    const effects: Effect[] = [];
    deps.forEach((effect) => effects.push(effect));
    for (let i = 0; i < effects.length; i++) {
      effects[i]();
    }
  }

  // Register an effect to run when dependencies change
  effect(fn: Effect) {
    this.currentEffect = fn;
    fn(); // Run immediately to collect dependencies
  }

  // Create a computed value that auto-updates when dependencies change
  computed<T>(getter: ComputedGetter<T>): ComputedGetter<T> {
    const self = this;

    // Mark as dirty initially
    this.computedDirty.set(getter, true);
    this.computedDeps.set(getter, new Set());

    // Create the invalidation effect
    const invalidationEffect = () => {
      self.computedDirty.set(getter, true);

      // Trigger all effects that depend on this computed
      const deps = self.computedDeps.get(getter);
      if (deps) {
        const effects: Effect[] = [];
        deps.forEach((effect) => effects.push(effect));
        for (let i = 0; i < effects.length; i++) {
          effects[i]();
        }
      }
    };

    // Wrap the getter to add caching and reactivity
    const wrappedGetter = (): T => {
      // Track this computed as a dependency for the current effect
      if (self.currentEffect !== invalidationEffect) {
        const deps = self.computedDeps.get(getter);
        if (deps) {
          deps.add(self.currentEffect);
        }
      }

      // If dirty, recompute
      if (self.computedDirty.get(getter)) {
        const previousEffect = self.currentEffect;

        // Set up tracking for dependencies - when dependencies change, mark as dirty
        self.currentEffect = invalidationEffect;

        // Run getter to collect dependencies and cache result
        const value = getter();
        self.computedCache.set(getter, value);
        self.computedDirty.set(getter, false);

        // Restore previous effect
        self.currentEffect = previousEffect;
      }

      return self.computedCache.get(getter);
    };

    return wrappedGetter;
  }
}

// ============================================================================
// Demo 1: Basic Reactivity
// ============================================================================

console.log("╔═══════════════════════════════════════╗");
console.log("║  Reactive State Demo for Paserati     ║");
console.log("╚═══════════════════════════════════════╝\n");

const system = new ReactiveSystem();

const state = system.reactive({
  count: 0,
  username: "Alice",
  theme: "dark",
});

// Effect 1: Log whenever username changes
system.effect(() => {
  console.log(`[User] Current user: ${state.username}`);
});

// Effect 2: Log count
system.effect(() => {
  console.log(`[Counter] Count: ${state.count}`);
});

// Effect 3: Log theme
system.effect(() => {
  console.log(`[Theme] Current theme: ${state.theme}`);
});

console.log("\n--- Incrementing count ---");
state.count = 1;
state.count = 2;
state.count = 3;

console.log("\n--- Changing theme ---");
state.theme = "light";

console.log("\n--- Changing username ---");
state.username = "Bob";

// ============================================================================
// Demo 2: Shopping Cart
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║      Shopping Cart Demo               ║");
console.log("╚═══════════════════════════════════════╝\n");

const cart = system.reactive({
  itemPrice: 10,
  quantity: 2,
  taxRate: 0.1,
});

system.effect(() => {
  const subtotal = cart.itemPrice * cart.quantity;
  const total = subtotal + subtotal * cart.taxRate;
  console.log(
    `[Cart] ${cart.quantity} × $${cart.itemPrice} = $${subtotal.toFixed(
      2
    )} (+ tax: $${total.toFixed(2)})`
  );
});

console.log("\n--- Adding more items ---");
cart.quantity = 5;

console.log("\n--- Price increase ---");
cart.itemPrice = 15;

console.log("\n--- Tax rate change ---");
cart.taxRate = 0.2;

// ============================================================================
// Demo 3: Computed Properties
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║     Computed Properties Demo          ║");
console.log("╚═══════════════════════════════════════╝\n");

const store = system.reactive({
  price: 100,
  quantity: 3,
  discount: 0.1,
  taxRate: 0.08,
});

// Computed: Subtotal
const subtotal = system.computed(() => {
  const result = store.price * store.quantity;
  console.log(`  [Computed] Calculating subtotal: ${store.price} × ${store.quantity} = ${result}`);
  return result;
});

// Computed: Discounted price (depends on subtotal)
const afterDiscount = system.computed(() => {
  const sub = subtotal();
  const result = sub * (1 - store.discount);
  console.log(`  [Computed] Applying discount: ${sub} × ${1 - store.discount} = ${result}`);
  return result;
});

// Computed: Final total (depends on afterDiscount)
const total = system.computed(() => {
  const discounted = afterDiscount();
  const result = discounted * (1 + store.taxRate);
  console.log(`  [Computed] Adding tax: ${discounted} × ${1 + store.taxRate} = ${result}`);
  return result;
});

// Effect that uses computed values
system.effect(() => {
  console.log(`[Total] Final price: $${total().toFixed(2)}`);
});

console.log("\n--- Increasing quantity ---");
store.quantity = 5;

console.log("\n--- Changing price ---");
store.price = 120;

console.log("\n--- Increasing discount ---");
store.discount = 0.2;

console.log("\n--- Reading cached computed values ---");
console.log(`Subtotal: $${subtotal().toFixed(2)} (cached, no recalc)`);
console.log(`After discount: $${afterDiscount().toFixed(2)} (cached, no recalc)`);
console.log(`Total: $${total().toFixed(2)} (cached, no recalc)`);

// ============================================================================
// Demo 4: Multiple Reactive Objects
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║     Multiple Objects Demo             ║");
console.log("╚═══════════════════════════════════════╝\n");

const user = system.reactive({
  name: "Alice",
  score: 0,
});

const game = system.reactive({
  level: 1,
  difficulty: "easy",
});

system.effect(() => {
  console.log(
    `[Game] ${user.name} is on level ${game.level} (${game.difficulty})`
  );
});

system.effect(() => {
  console.log(`[Score] ${user.name} has ${user.score} points`);
});

console.log("\n--- Playing the game ---");
user.score = 100;
game.level = 2;
user.score = 250;
game.difficulty = "medium";
user.name = "Bob";

// ============================================================================
// Demo 5: Conditional Effects
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║      Conditional Effects Demo         ║");
console.log("╚═══════════════════════════════════════╝\n");

const settings = system.reactive({
  temperature: 20,
  unit: "C",
});

system.effect(() => {
  const temp = settings.temperature;
  const unit = settings.unit;

  if (unit === "C") {
    console.log(`[Temp] Current temperature: ${temp}°C`);
  } else {
    const fahrenheit = temp * 1.8 + 32;
    console.log(`[Temp] Current temperature: ${fahrenheit.toFixed(1)}°F`);
  }
});

console.log("\n--- Increasing temperature ---");
settings.temperature = 25;

console.log("\n--- Switching to Fahrenheit ---");
settings.unit = "F";

console.log("\n--- Lowering temperature ---");
settings.temperature = 15;

// ============================================================================
// Summary
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║         Demo Complete!                ║");
console.log("╚═══════════════════════════════════════╝\n");

console.log("✨ This demo showcased:");
console.log("  • Proxy and Reflect for reactive tracking");
console.log("  • Computed properties with automatic caching");
console.log("  • Derived values with dependency chains");
console.log("  • Generic types with classes");
console.log("  • Map and Set collections");
console.log("  • Closures and higher-order functions");
console.log("  • Automatic dependency tracking");
console.log("  • Multiple reactive objects");
console.log("  • Conditional reactive effects");
console.log("  • Modern TypeScript/ES6+ features");
console.log("\nAll running natively in Paserati! 🚀\n");
