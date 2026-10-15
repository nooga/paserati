// expect: hi 2
// An interface extending a class gets the members the class inherits too:
// undici's Response extends BodyMixin, which declares body.
declare class BodyMixin { readonly body: string }
declare class Res extends BodyMixin { readonly status: number }
interface R extends Res {}
type Alias = Res;
interface R2 extends Alias {}
const r: R = { body: "hi", status: 2 };
const r2: R2 = r;
`${r2.body} ${r.status}`;
