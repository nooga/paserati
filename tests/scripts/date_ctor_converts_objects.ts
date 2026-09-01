// expect: 0:0:1234:true:true
// new Date(x) with a single non-Date argument must go through ToPrimitive
// before deciding whether to parse a string or run ToNumber (spec 21.4.2.1
// step 3). It used to call x.ToString() directly for object arguments, so
// new Date({valueOf: () => 0}) tried to parse "[object Object]" as a date, and
// the unparseable case stored float64(0x7FF8000000000000) - the NaN bit pattern
// converted as an *integer*, ~9.22e18 - instead of a NaN, reporting a huge
// timestamp rather than an Invalid Date.
const viaValueOf = new Date({ valueOf: () => 0 } as any).getTime();
const viaWrapper = new Date(Object(0) as any).getTime();
const viaDateCopy = new Date(new Date(1234)).getTime();

// An object with no numeric conversion is an Invalid Date, not a huge number.
const plainObjectIsInvalid = Number.isNaN(new Date({} as any).getTime());
// ToNumber, not date-string parsing, for non-string primitives.
const booleanIsOne = new Date(true as any).getTime() === 1;

viaValueOf + ":" + viaWrapper + ":" + viaDateCopy + ":" +
  plainObjectIsInvalid + ":" + booleanIsOne;
