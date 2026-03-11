// Helper module emulating a driver-like API with type predicates,
// mapped types, and sequential narrowing patterns.

export interface ValueNode {
  kind: "value";
  get?: () => unknown;
  value?: unknown;
}

export interface CallNode {
  kind: "call";
  fn(payload: unknown): unknown;
}

export type DriverNode = ValueNode | CallNode;
export type NodeMap = Record<string, DriverNode>;

export function isValueNode(node: unknown): node is ValueNode {
  return typeof node === "object" && node !== null && (node as any).kind === "value";
}

export function isCallNode(node: unknown): node is CallNode {
  return typeof node === "object" && node !== null && (node as any).kind === "call";
}

export function resolveNode(map: NodeMap, key: string): DriverNode | null {
  if (map[key] !== undefined) {
    return map[key] as DriverNode;
  }
  return null;
}

export function readNode(map: NodeMap, key: string): unknown {
  const node: DriverNode | null = resolveNode(map, key);
  if (node == null) {
    return null;
  }
  // After null check, node is: DriverNode
  if (!isValueNode(node)) {
    return null;
  }
  // After type predicate narrowing, node is: ValueNode
  if (typeof node.get === "function") {
    return node.get();
  }
  return node.value;
}
