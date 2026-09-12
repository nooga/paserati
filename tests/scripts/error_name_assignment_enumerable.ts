// expect: true
// Regression test for #420: Error instances have no own "name" property, so
// `err.name = "X"` creates an ordinary enumerable own property that survives
// JSON.stringify / Object.keys. "message" is own (non-enumerable) only when
// a message argument was actually supplied.
const e = new Error("msg");
(e as any).name = "MyName";
const d = Object.getOwnPropertyDescriptor(e, "name")!;

class MyError extends Error {
  constructor(msg: string) {
    super(msg);
    this.name = "MyError";
  }
}
const m = new MyError("oops");

const t = new TypeError("tt");
(t as any).name = "Renamed";

const results = [
  d.enumerable === true && d.writable === true && d.configurable === true,
  JSON.stringify(e) === '{"name":"MyName"}',
  JSON.stringify(m) === '{"name":"MyError"}',
  Object.keys(m).join(",") === "name",
  String(m) === "MyError: oops",
  JSON.stringify(t) === '{"name":"Renamed"}',
  // No own "name" on fresh instances; inherited from the prototype instead.
  !new Error().hasOwnProperty("name") && new Error().name === "Error",
  !new RangeError().hasOwnProperty("name") && new RangeError().name === "RangeError",
  !new AggregateError([]).hasOwnProperty("name") && new AggregateError([]).name === "AggregateError",
  // Own "message" only when a message argument was supplied.
  !new Error().hasOwnProperty("message") && new Error().message === "",
  new Error("a").hasOwnProperty("message") && Object.getOwnPropertyDescriptor(new Error("a"), "message")!.enumerable === false,
  !new TypeError(undefined).hasOwnProperty("message"),
  // Error.prototype.toString reads inherited name/message.
  String(new SyntaxError("s")) === "SyntaxError: s",
  String(new AggregateError([], "agg")) === "AggregateError: agg",
];
results.every((r) => r);
