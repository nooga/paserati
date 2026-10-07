// expect: threw boom|threw boom|outer|threw boom
function g(): void { throw new Error("boom"); }
function a() { g(); try { } catch (e) { } return "continued"; }
function b() { const x = g(); try { } catch (e) { } return "continued"; }
function c() { try { g(); try { } catch (e) { return "inner"; } } catch (e) { return "outer"; } return "none"; }
function d() { g(); let z = 1; try { } catch (e) { } return "continued"; }
const out: string[] = [];
for (const f of [a, b, c, d]) {
  let r: string;
  try { r = f(); } catch (e) { r = "threw " + (e as Error).message; }
  out.push(r);
}
out.join("|");
