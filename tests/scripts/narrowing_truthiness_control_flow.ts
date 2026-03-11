// expect: hello
// Test: truthiness check control flow narrowing
// if (!x) { return } should narrow x to non-null after the return

function getValue(map: Record<string, string> | null, key: string): string {
  if (!map) {
    return "none";
  }
  // map should be narrowed to Record<string, string> (non-null)
  return map[key] ?? "missing";
}

const data: Record<string, string> = { greeting: "hello" };
getValue(data, "greeting");
