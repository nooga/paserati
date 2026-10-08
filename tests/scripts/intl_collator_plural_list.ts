// expect: aAäbzZ|aAbzZä|a1,a2,a10|0|one,two,few,other|one,few,many|a, b, and c|a or b
// Intl.Collator (and localeCompare), PluralRules and ListFormat (#624).
const letters = ["b", "a", "Z", "ä", "z", "A"];
[
  [...letters].sort(new Intl.Collator("en").compare).join(""),
  [...letters].sort(new Intl.Collator("sv").compare).join(""),
  ["a10", "a2", "a1"].sort(new Intl.Collator("en", { numeric: true }).compare).join(","),
  "a".localeCompare("á", "en", { sensitivity: "base" }),
  [1, 2, 3, 4].map((n) => new Intl.PluralRules("en", { type: "ordinal" }).select(n)).join(","),
  [1, 2, 5].map((n) => new Intl.PluralRules("pl").select(n)).join(","),
  new Intl.ListFormat("en").format(["a", "b", "c"]),
  new Intl.ListFormat("en", { type: "disjunction" }).format(["a", "b"]),
].join("|");
