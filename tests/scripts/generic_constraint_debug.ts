// Debug generic constraint resolution
// expect: success

function test<T extends (n: number) => number>(callback: (f: T) => T): T {
    const dummy = callback as any;
    return dummy;
}

type MyFunc = (n: number) => number;

const result = test<MyFunc>((f) => (n) => f(n));

"success";