// expect: ok|1|365.00
// #635: optional-chain truthiness, assignment to a property, truthy ternary.
interface Id { DkimAttributes?: { Tokens?: string[] } }
const identity: Id = { DkimAttributes: { Tokens: ["t"] } };
let a = "";
if (identity.DkimAttributes?.Tokens && identity.DkimAttributes.Tokens.length > 0) a = "ok";
interface St { n?: number; cost?: number }
const st: St = { cost: 0.5 };
st.n = (st.n || 0) + 1;
const b = st.n >= 5 ? "five" : st.n;
const rate = st.cost;
const c = rate ? (rate * 730).toFixed(2) : "unknown";
`${a}|${b}|${c}`;
