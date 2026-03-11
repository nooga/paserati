// expect: hello
// Test: compound || condition with throw narrows both sides
// Pattern: if (x == null || !isType(x)) { throw } should narrow x

interface ValueNode {
  kind: "value";
  get?: () => unknown;
  value?: unknown;
}

interface DirNode {
  kind: "dir";
}

type Node = ValueNode | DirNode;

function isValueNode(node: unknown): node is ValueNode {
  return typeof node === "object" && node !== null && (node as any).kind === "value";
}

function read(node: Node | null): unknown {
  // Combined null check + type predicate in single || condition
  if (node == null || !isValueNode(node)) {
    throw "not a value node";
  }
  // After throw: node is narrowed to ValueNode
  if (typeof node.get === "function") {
    return node.get();
  }
  return node.value;
}

const v: ValueNode = { kind: "value", value: "hello" };
read(v);
