// expect: MX|x high|clusters c1,flex c1|ab
// #636: for-of over `x || []`, `x ?? []`, nested loops over any, readonly arrays.
interface St {
  records?: Array<{ record?: string }>;
  candidates?: { id: string; availability: string }[];
}
const st: St = { records: [{ record: "mx" }], candidates: [{ id: "x", availability: "high" }] };
let r1 = "";
for (const r of st.records || []) r1 += (r?.record || "").toUpperCase();
let r2 = "";
for (const c of st.candidates ?? []) r2 += `${c.id} ${c.availability}`;
function req(): any { return { results: [{ name: "c1" }] }; }
const out: string[] = [];
for (const collection of ["clusters", "flex"]) {
  const response = req();
  const clusters = response && response.results ? response.results : [];
  for (const cluster of clusters) out.push(`${collection} ${cluster.name}`);
}
const ro: ReadonlyArray<string> | undefined = ["a", "b"];
let r4 = "";
for (const s of ro ?? []) r4 += s;
`${r1}|${r2}|${out.join(",")}|${r4}`;
