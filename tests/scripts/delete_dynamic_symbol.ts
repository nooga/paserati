// expect: true
// Verify delete with dynamic symbol key respects [[Configurable]]
const s = Symbol("x");
const obj: any = {};
Object.defineProperty(obj, s, {
  value: 1,
  configurable: true,
  enumerable: true,
  writable: true,
});
// Dynamic delete (computed key); yield the value as script result
const result = delete (obj as any)[s];
result;
