// expect: hello
type Fn = (...args: [...string[]]) => void;
type Fn2 = (...args: [string, ...string[]]) => void;
"hello";
