// Test generic constraints

interface Lengthable {
    length: number;
}

type HasLength<T extends Lengthable> = T;
interface Container<T extends Lengthable> {
    item: T;
    getLength(): number;
}

let stringContainer: Container<string>;
let arrayContainer: Container<Array<number>>;

stringContainer;
arrayContainer;

// expect: undefined