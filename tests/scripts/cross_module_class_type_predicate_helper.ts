// Helper: class with methods that use type predicate functions for narrowing

export interface ValueNode {
  kind: "value";
  get?: () => unknown;
  value?: unknown;
}

export interface DirNode {
  kind: "dir";
}

export type DriverNode = ValueNode | DirNode;

export function isValueNode(node: unknown): node is ValueNode {
  return typeof node === "object" && node !== null && (node as any).kind === "value";
}

export class BaseReader {
  protected nodes: Record<string, DriverNode> = {};

  read(key: string): unknown {
    const node: DriverNode | undefined = this.nodes[key];
    if (node == null) {
      return null;
    }
    if (!isValueNode(node)) {
      return null;
    }
    // node narrowed to ValueNode
    if (typeof node.get === "function") {
      return node.get();
    }
    return node.value;
  }
}
