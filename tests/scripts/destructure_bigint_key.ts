// expect: FIXME bigint key destructuring needs compiler work
let { 1n: a } = { "1": "foo" };
a;
