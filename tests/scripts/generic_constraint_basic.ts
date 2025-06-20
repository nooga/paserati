// Test generic constraints

interface Lengthable {
    length: number;
}

type HasLength<T extends Lengthable> = T;
interface Container<T extends Lengthable> {
    item: T;
    getLength(): number;
}

// Use object types that satisfy the constraint
// Note: Built-in types like string/Array don't work due to structural typing limitations
let objContainer: Container<{length: number; value: string}>;

objContainer;

// expect: undefined