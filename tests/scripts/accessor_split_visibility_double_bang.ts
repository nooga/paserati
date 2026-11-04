// expect_compile_error: Property 'ws' is private and only accessible within class 'Conn'.
// `!!this.x` narrows like `this.x`; a public getter with a private setter
// reads publicly and writes only inside the class.
class Conn {
  private _ws: { readyState: number } | null = null;
  public get ws() { return this._ws; }
  private set ws(ws) { this._ws = ws; }
  isOpen(): boolean { return !!this.ws && this.ws.readyState === 1; }
  reset() { this.ws = null; }
}
function read(c: Conn) { return c.ws; }
function write(c: Conn) { c.ws = null; }
