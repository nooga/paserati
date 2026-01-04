// Test explicit type arguments with mapped type containing tuple values
// expect: x

// Mapped type with tuple values
type TupleRecord<K extends string, A, B> = { [P in K]: [A, B] };

function getFirstKey<K extends string, A, B>(obj: TupleRecord<K, A, B>): K {
  return Object.keys(obj)[0] as K;
}

// With explicit type args, contextual typing flows to tuple values
const key = getFirstKey<"x" | "y", number, string>({
  x: [1, "hello"],
  y: [2, "world"]
});

key;
