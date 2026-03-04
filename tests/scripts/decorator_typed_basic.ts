// expect: wrapped:hello
// Decorator with explicit typed context (not any, not Function)
function wrap<T extends (...args: any[]) => any>(
  target: T,
  context: { kind: string; name: string }
): T {
  const replacement = function(this: any, ...args: any[]) {
    return "wrapped:" + target.apply(this, args);
  };
  return replacement as unknown as T;
}

class Greeter {
  @wrap
  greet(): string {
    return "hello";
  }
}

const g = new Greeter();
g.greet();
