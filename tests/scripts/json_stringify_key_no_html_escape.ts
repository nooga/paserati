// expect: {"a<b":1,"c&d":2,"e>f":3,"\ud800x":5,"q\"\n":6}|{ "a<b": 1}|true true|["a<b","\ud800x"]
// skip-typecheck
// paserati#516: object keys were quoted with Go's json.Marshal, which
// HTML-escapes <, >, & and U+2028/U+2029 and mangles lone surrogates. Keys
// must use the same QuoteJSONString as string values.
const k = { "a<b": 1, "c&d": 2, "e>f": 3, "\ud800x": 5, 'q"\n': 6 };
const ls = JSON.stringify({ "g h ": 1 });
[
  JSON.stringify(k),
  JSON.stringify({ "a<b": 1 }, null, 1).replace(/\n/g, ""),
  ls.includes(" ") + " " + ls.includes(" "),
  JSON.stringify(["a<b", "\ud800x"]),
].join("|");
