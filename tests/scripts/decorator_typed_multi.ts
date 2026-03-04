// expect: validated:cached:compute(5)=25
// Multiple properly typed decorators stacked: validation + caching
interface MethodContext {
  kind: string;
  name: string;
  addInitializer: (fn: () => void) => void;
}

function cached<T extends (...args: any[]) => any>(
  target: T,
  context: MethodContext
): T {
  const cache: any = {};
  const replacement = function(...args: any[]): any {
    const key = args.join(",");
    if (cache[key] !== undefined) {
      return cache[key];
    }
    const result = (target as any).apply(null, args);
    cache[key] = "cached:" + result;
    return cache[key];
  };
  return replacement as unknown as T;
}

function validated<T extends (...args: any[]) => any>(
  target: T,
  context: MethodContext
): T {
  const replacement = function(...args: any[]): any {
    for (const arg of args) {
      if (typeof arg !== "number") {
        throw new Error("Expected number argument");
      }
    }
    const result = (target as any).apply(null, args);
    return "validated:" + result;
  };
  return replacement as unknown as T;
}

class MathService {
  @validated
  @cached
  compute(n: number): string {
    return "compute(" + n + ")=" + (n * n);
  }
}

const math = new MathService();
math.compute(5);
