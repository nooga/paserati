// Test basic generic type alias declaration and usage

type Optional<T> = T | undefined;
type Pair<T, U> = { first: T; second: U };
type Callback<T, R> = (arg: T) => R;

let maybeString: Optional<string>;
let numberPair: Pair<number, string>;  
let stringToNumber: Callback<string, number>;

maybeString;
numberPair;
stringToNumber;

// expect: undefined