// expect: 1 -1 -128 127 255 18446744073709551615 0 5 777 0 TypeError SyntaxError RangeError
// #610: BigInt.asUintN / asIntN wrap; StringToBigInt is decimal unless prefixed
const r: string[] = [
  BigInt.asUintN(8, 257n),
  BigInt.asIntN(8, 255n),
  BigInt.asIntN(8, 128n),
  BigInt.asIntN(8, -129n),
  BigInt.asUintN(8, -1n),
  BigInt.asUintN(64, -1n),
  BigInt.asIntN(0, 5n),
  BigInt.asUintN(3, "13" as any),
].map(String);
r.push(String(BigInt("0777")), String(BigInt("")));
try { BigInt.asUintN(3, 13 as any); } catch (e: any) { r.push(e.name); }
try { BigInt("1_000"); } catch (e: any) { r.push(e.name); }
try { BigInt.asIntN(-1, 1n); } catch (e: any) { r.push(e.name); }
r.join(" ");
