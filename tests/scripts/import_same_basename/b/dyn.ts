export async function dynB(x: number): Promise<number> {
  const m = await import("./_helper.ts");
  return m.transform(x);
}
export async function dynOnlyB(): Promise<string> {
  const m = await import("./_helper.ts");
  return m.onlyInB();
}
