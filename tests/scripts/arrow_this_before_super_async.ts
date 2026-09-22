// expect: 17,gen
// skip-typecheck
// paserati#520: an async arrow created before super() and resumed after it
// sees the bound `this` - its frame is restored from the saved promise state,
// so the shared binding has to be reached through the closure.
class Base { constructor(x) { this.x = x; } }
class AA extends Base {
  constructor() {
    const g = async () => { await null; return this.x; };
    const h = async () => { await null; return this.y; };
    super(17);
    this.y = "gen";
    this.p = Promise.all([g(), h()]);
  }
}
(await new AA().p).join();
