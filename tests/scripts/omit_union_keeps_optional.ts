// expect: idle
// Omit (Pick) over a union copies the union's property modifiers: a member
// optional in some constituent stays optional.
interface Base<T> { mutate: (x: T) => void }
interface Succ<T> extends Base<T> { data: T; error?: never; status: "success" }
interface Idle<T> extends Base<T> { data?: never; error?: never; status: "idle" }
interface Err<T> extends Base<T> { data?: never; error: Error; status: "error" }
type State = Omit<Succ<number> | Idle<number> | Err<number>, "mutate">;
const s: State = { status: "idle" };
s.status;
