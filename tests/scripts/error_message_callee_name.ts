// expect: Cannot read properties of undefined (reading 'forEach')|o.run is not a function|f is not a function|o.k is not a function|o.y is not a function
// #622: "is not a function" names the callee as written, like V8
const o: any = {};
const msgs: string[] = [];
try { o.items.forEach((x: any) => x); } catch (e: any) { msgs.push(e.message); }
try { o.run(); } catch (e: any) { msgs.push(e.message); }
try { const f = o.cb; f(); } catch (e: any) { msgs.push(e.message); }
try { o["k"](); } catch (e: any) { msgs.push(e.message); }
try { o.x(o.y()); } catch (e: any) { msgs.push(e.message); }
msgs.join("|");
