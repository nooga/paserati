// expect: 7
// Generic method decorator that preserves args/return types
function logged<Args extends any[], Return>(
  target: (...args: Args) => Return,
  context: { kind: string; name: string }
): (...args: Args) => Return {
  const methodName = context.name;
  return function(...args: Args): Return {
    const result = target.apply(null, args);
    console.log("[logged] " + methodName + "(" + args.join(", ") + ") => " + result);
    return result;
  };
}

class Calculator {
  @logged
  compute(a: number, b: number): number {
    return a + b;
  }
}

const calc = new Calculator();
calc.compute(3, 4);
