// expect: 72,101,108,108,111
// Spreading a TypedArray into an array literal or call arguments (paserati#498)

const u8 = new Uint8Array([72, 101, 108, 108, 111]);
const spread = [...u8];
const s = String.fromCharCode(...u8);
`${spread.join(",")}` === "72,101,108,108,111" && s === "Hello" ? spread.join(",") : "FAIL";
