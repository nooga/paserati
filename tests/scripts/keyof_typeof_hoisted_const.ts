// expect: BAD_REQUEST
// `keyof typeof C` in an alias declared (and used in a signature) before
// C's initializer is checked keys C once it has a type, not never.
type Code = keyof typeof CODES;
class Failure {
  code: Code;
  constructor(opts: { code: Code }) { this.code = opts.code; }
}
const CODES = { PARSE_ERROR: -32700, BAD_REQUEST: -32600 } as const;
const k: Code = "PARSE_ERROR";
new Failure({ code: "BAD_REQUEST" }).code;
