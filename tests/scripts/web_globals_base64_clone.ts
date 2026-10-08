// expect: aGVsbG8=|hello|InvalidCharacterError|true|true|5|2|1,2|true|bad|DataCloneError
// #624: atob/btoa and structuredClone
const out: any[] = [btoa("hello"), atob("aGVs bG8")];
try { btoa("✓"); } catch (e: any) { out.push(e.name); }
const src: any = { d: new Date(5), m: new Map([[1, { k: 2 }]]), s: new Set([1, 2]), e: new TypeError("bad") };
src.self = src;
const c = structuredClone(src);
out.push(c !== src && c.self === c, c.d instanceof Date, c.d.getTime(), c.m.get(1).k, [...c.s].join(), c.e instanceof TypeError, c.e.message);
try { structuredClone(() => 1); } catch (e: any) { out.push(e.name); }
out.join("|");
