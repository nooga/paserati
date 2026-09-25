// AbortSignal dispatches a real, trusted Event; stopImmediatePropagation
// skips the remaining listeners and onabort (#567).
// expect: [object Event],abort,true,true,true,2,skipped,null,0
const ac = new AbortController();
const log: any[] = [];
let ran = false;
ac.signal.addEventListener("abort", (e: Event) => {
  log.push(String(e), e.type, e instanceof Event, e.isTrusted, e.target === ac.signal, e.eventPhase);
  e.stopImmediatePropagation();
});
ac.signal.addEventListener("abort", () => { ran = true; });
ac.signal.onabort = () => { ran = true; };
let seen: Event | undefined;
ac.signal.addEventListener("abort", (e: Event) => { seen = e; }, { once: true });
ac.abort();
log.push(ran ? "ran" : "skipped");
const e = new Event("x");
log.push(e.currentTarget, e.eventPhase);
log.map(String).join(",");
