// expect: 36
// A property of an any value stays assignable after an equality guard
// narrows it (#706).
const o: any = { z: null };
if (o.x === undefined) o.x = 5;
if (o.y == null) o.y = 6;
if (o.z === null) o.z = 7;
const opts: any = { nested: {} };
if (opts.nested.timeout == null) opts.nested.timeout = 18;
o.x + o.y + o.z + opts.nested.timeout;
