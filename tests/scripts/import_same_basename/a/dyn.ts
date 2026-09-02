export async function dynA(x: number): Promise<number> {
  const m = await import("./_helper.ts");
  return m.transform(x);
}
export async function dynOnlyA(): Promise<string> {
  const m = await import("./_helper.ts");
  return m.onlyInA();
}
