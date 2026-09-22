// Helper for import_shadowed_by_nested_param.ts; not a test on its own.
export function resolveHelper(...args: number[]) {
  return "MODULE:" + args.join(",");
}
