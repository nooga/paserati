// The Event constructor: EventInit, preventDefault/cancelable, subclassing.
// expect: x,true,false,false,false|true,false|false|my,true,1|TypeError,TypeError
const log: string[] = [];
const e = new Event("x", { bubbles: true });
log.push([e.type, e.bubbles, e.cancelable, e.composed, e.isTrusted].join(","));
const c = new Event("c", { cancelable: true });
c.preventDefault();
log.push([c.defaultPrevented, c.returnValue].join(","));
e.preventDefault();
log.push(String(e.defaultPrevented));
class MyEvent extends Event {
  extra: number;
  constructor() { super("my"); this.extra = 1; }
}
const m = new MyEvent();
log.push([m.type, m instanceof Event, m.extra].join(","));
const errs: string[] = [];
try { (Event as any)("x"); } catch (err) { errs.push((err as Error).constructor.name); }
try { new (Event as any)(); } catch (err) { errs.push((err as Error).constructor.name); }
log.push(errs.join(","));
log.join("|");
