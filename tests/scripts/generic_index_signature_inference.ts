// expect: 42

interface StringIndex<T> {
    [s: string]: T;
}

interface NumberIndex<T> {
    [n: number]: T;
}

declare function acceptStringIndex<T>(obj: StringIndex<T>): T;
declare function acceptNumberIndex<T>(obj: NumberIndex<T>): T;

if (false) {
    acceptStringIndex({
        p: "",
        0: () => { },
        ["hi" + "bye"]: true,
        [0 + 1]: 0,
        [+"hi"]: [0],
    });

    acceptNumberIndex({
        0: () => { },
        [0 + 1]: 0,
    });
}

42;
