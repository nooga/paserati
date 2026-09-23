// expect: sliceHoles=[[["0","2"],3],["0","2998"],2999,[9]] ; sliceEmpty=[[],3] ; sliceArrayLike=[["0","2"],4] ; sortHoles=[[["0","1","2"],11],[1,2,3,null]] ; sortUndef=[[["0","1","2","3","4"],6],[1,2,3,null,null,null]] ; sortUndefCmp=[[1,2,null,null],true] ; badCmp="!TypeError" ; badCmpNull="!TypeError" ; cmpNaN=[3,1,2] ; cmpFrac=[1,2,3] ; cmpValueOf=[1,2,3] ; utf16=[97,128512,64256] ; numbers=[1,10,100,9] ; stable="bdac" ; throwCmp="!Error:boom" ; throwToString="!Error:boom" ; frozenSort="!TypeError" ; arrayLike={"0":"a","1":"b","2":"c","length":4} ; toSortedHoles=[[["0","1","2"],3],[1,3,null]] ; toSortedUndef=[1,2,null] ; toSortedBad="!TypeError" ; toSortedFrac=[1,2,3] ; big=[0,999,1999] ; proxy=[1,2,[]]
// skip-typecheck
// paserati#547: slice keeps holes as holes, and sort/toSorted follow
// SortIndexedProperties - holes skipped and deleted past the sorted values,
// undefined last without calling comparefn, a non-callable comparefn
// rejected, comparefn results through ToNumber, UTF-16 string order, a
// stable O(n log n) sort, and write-back through Set/DeletePropertyOrThrow.
const out = [];
const T = (n, f) => { let r; try { r = f(); } catch (e) { r = "!" + e.constructor.name + (e instanceof Error && e.message === "boom" ? ":boom" : ""); } out.push(n + "=" + JSON.stringify(r)); };
const K = (a) => [Object.keys(a), a.length];
T("sliceHoles", () => { const a = [1, , 3]; a[3000] = 9; return [K(a.slice(0, 3)), Object.keys(a.slice(2)), a.slice(2).length, a.slice(-1)]; });
T("sliceEmpty", () => K([, , ,].slice()));
T("sliceArrayLike", () => K(Array.prototype.slice.call({ length: 4, 0: "a", 2: "c" })));
T("sortHoles", () => { const c = [3, , 1]; c[10] = 2; c.sort(); return [K(c), c.slice(0, 4)]; });
T("sortUndef", () => { const a = [3, undefined, 1, , undefined, 2]; a.sort(); return [K(a), a]; });
T("sortUndefCmp", () => { let calls = 0; const a = [undefined, 2, undefined, 1]; a.sort((x, y) => { calls++; if (x === undefined || y === undefined) throw 1; return x - y; }); return [a, calls > 0]; });
T("badCmp", () => [1, 2].sort(1));
T("badCmpNull", () => [1, 2].sort(null));
T("cmpNaN", () => [3, 1, 2].sort(() => NaN));
T("cmpFrac", () => [3, 1, 2].sort((a, b) => (a - b) / 10));
T("cmpValueOf", () => [3, 1, 2].sort((a, b) => ({ valueOf() { return a - b; } })));
T("utf16", () => ["😀", "ﬀ", "a"].sort().map(s => s.codePointAt(0)));
T("numbers", () => [10, 9, 1, 100].sort());
T("stable", () => [{ k: 1, v: "a" }, { k: 0, v: "b" }, { k: 1, v: "c" }, { k: 0, v: "d" }].sort((x, y) => x.k - y.k).map(o => o.v).join(""));
T("throwCmp", () => [2, 1].sort(() => { throw new Error("boom"); }));
T("throwToString", () => [{ toString() { throw new Error("boom"); } }, 1].sort());
T("frozenSort", () => Object.freeze([2, 1]).sort());
T("arrayLike", () => { const o = { length: 4, 0: "c", 1: "a", 3: "b" }; Array.prototype.sort.call(o); return o; });
T("toSortedHoles", () => { const t = [3, , 1].toSorted(); return [K(t), t]; });
T("toSortedUndef", () => [undefined, 2, 1].toSorted());
T("toSortedBad", () => [1].toSorted(1));
T("toSortedFrac", () => [3, 1, 2].toSorted((a, b) => (a - b) / 10));
T("big", () => { const a = []; for (let i = 0; i < 2000; i++) a.push((i * 7919) % 2000); a.sort((x, y) => x - y); return [a[0], a[999], a[1999]]; });
T("proxy", () => { const log = []; const p = new Proxy([2, 1], { deleteProperty(t, k) { log.push("del" + k); return Reflect.deleteProperty(t, k); } }); Array.prototype.sort.call(p); return [p[0], p[1], log]; });
out.join(" ; ");
