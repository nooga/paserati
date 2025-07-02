// Test the exact pattern from date-fns with built-in types

export interface AddOptions<DateType extends object = {}> {
    prop: DateType;
}

export function test<T extends string = "default">(param: T): T {
    return param;
}

test;
// expect: [Function: test]