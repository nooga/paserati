// expect: true
// Test: && expression narrowing carries type predicate to right operand

function isObjectRecord(input: unknown): input is Record<string, unknown> {
  return typeof input === "object" && input !== null;
}

function isCallNode(node: unknown): boolean {
  // isObjectRecord narrows node, so node["fn"] should be valid
  return isObjectRecord(node) && typeof node["fn"] === "function";
}

const obj = { fn: () => 42 };
isCallNode(obj);
