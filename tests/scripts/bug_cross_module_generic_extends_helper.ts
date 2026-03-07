export abstract class Base<S, R> {
  state: S = undefined as any;
  abstract step(s: S): S;
  abstract finalize(s: S): R;
}
