// expect: nearline|true
type Tier = "STANDARD" | "NEARLINE" | "COLDLINE";
const t: Tier = "NEARLINE" as Tier;
[t.toLowerCase(), t.includes("LINE")].join("|");
