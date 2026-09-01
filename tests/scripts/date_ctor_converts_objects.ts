// expect: 0:0:1234:true:true:true
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

// new Date(string) and Date.parse must accept the same formats (spec 21.4.3.2).
// The constructor used to carry a shorter ladder of its own, so these two were
// Invalid Date through the constructor while Date.parse handled them.
const parseAgrees = ["01/02/2006", "January 2, 2006", "2006-01-02T15:04:05Z", "2006", "nope"]
  .every((s) => {
    const viaParse = Date.parse(s);
    const viaCtor = new Date(s).getTime();
    return viaParse === viaCtor ||
      (Number.isNaN(viaParse) && Number.isNaN(viaCtor));
  });

viaValueOf + ":" + viaWrapper + ":" + viaDateCopy + ":" +
  plainObjectIsInvalid + ":" + booleanIsOne + ":" + parseAgrees;
