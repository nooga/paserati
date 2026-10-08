// expect: true|p|{"a":1,"b":2}|1
// #634: Record<string, T> works in `in`, `?.`, spread and intersections.
const newTags: Record<string, string> = { a: "1" };
const hasA = "a" in newTags;
function g(args?: Record<string, string>): string {
  return (args?.prefix as string) || "";
}
const properties = { a: 1 } as Record<string, unknown>;
const spread = JSON.stringify({ ...properties, b: 2 });
type R = { Items?: Array<Record<string, any>>; Name?: string } & Record<string, any>;
const res: R = { items: [1] };
`${hasA}|${g({ prefix: "p" })}|${spread}|${(res.items || res.Items || []).length}`;
