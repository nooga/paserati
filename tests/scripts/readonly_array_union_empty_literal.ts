// expect: 2
// An array literal takes its context from the one array member of a union
// (`ReadonlyArray<T> | null = []`, tRPC's vendored Unpromise).
type RA = ReadonlyArray<number>;
let subscribers: RA | null = [];
const items: ReadonlyArray<string> | undefined = ["a", "b"];
subscribers = [...(subscribers ?? []), 1];
subscribers.length + (items?.length ?? 0) - 1;
