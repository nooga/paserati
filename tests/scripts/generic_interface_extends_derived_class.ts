// expect: function
class Readable { pipe(): void {} }
class IM extends Readable { headers = ""; }
interface H<P> extends IM { foo: P }
const h: H<number> = Object.assign(new IM(), { foo: 1 });
typeof h.pipe;
