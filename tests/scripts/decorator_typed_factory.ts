// expect: [TAG] 42
// Decorator factory returning a properly typed decorator
interface DecoratorContext {
  kind: string;
  name: string;
}

function tag(label: string): <T extends (...args: any[]) => any>(
  target: T,
  context: DecoratorContext
) => T {
  return function<T extends (...args: any[]) => any>(
    target: T,
    context: DecoratorContext
  ): T {
    const replacement = function(...args: any[]): any {
      const result = (target as any).apply(null, args);
      return "[" + label + "] " + result;
    };
    return replacement as unknown as T;
  };
}

class Service {
  @tag("TAG")
  getValue(): string {
    return "42";
  }
}

const svc = new Service();
svc.getValue();
