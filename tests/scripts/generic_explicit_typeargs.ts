// Test explicit type arguments in generic function calls
// expect: hello

// Simple generic function with explicit type arg
function identity<T>(x: T): T { return x; }

const r1 = identity<string>("hello");
const r2 = identity<number>(42);

// Mapped type with explicit type args
type MyRecord<K extends string, V> = { [P in K]: V };

function getKey<K extends string, V>(obj: MyRecord<K, V>): K {
  return Object.keys(obj)[0] as K;
}

const k = getKey<"a" | "b", number>({ a: 1, b: 2 });

// Return first result to verify explicit type args work
r1;
