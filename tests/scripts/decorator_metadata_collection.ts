// Test decorator metadata collection pattern (used by @tool in anjin)
// Decorators push metadata to a module-level array, then the constructor reads it

const metadata: { name: string; desc: string }[] = [];

function tool(description: string) {
  return function(method: any, context: any): any {
    metadata.push({ name: context.name, desc: description });
    return method;
  };
}

class Worker {
  @tool("does alpha")
  alpha(): string { return "a"; }

  @tool("does beta")
  beta(): string { return "b"; }

  getTools(): string {
    return metadata.map((m: any) => m.name).join(",");
  }
}

const w = new Worker();
w.getTools();

// expect: alpha,beta
