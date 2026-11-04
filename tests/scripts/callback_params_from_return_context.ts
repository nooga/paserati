// expect: 3
// A callback argument's parameters are typed by what the call's expected
// return type infers for type parameters nothing else determines.
interface Observer<TValue, TError> { next: (value: TValue) => void; error: (err: TError) => void; }
interface Observable<TValue, TError> { subscribe(opts: Partial<Observer<TValue, TError>>): void; }
function observable<TValue, TError = unknown>(subscribe: (observer: Observer<TValue, TError>) => void): Observable<TValue, TError> {
  return { subscribe(o) { subscribe({ next: (v) => o.next?.(v), error: (e) => o.error?.(e) }); } };
}
type Res<T> = Observable<{ r: T }, Error>;
function chain<T>(src: Res<T>): Res<T> {
  return observable((observer) => { src.subscribe(observer); });
}
let got = 0;
chain(observable<{ r: number }, Error>((o) => o.next({ r: 3 }))).subscribe({ next: (v) => { got = v.r; } });
got;
