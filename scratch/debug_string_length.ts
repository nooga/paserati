interface Lengthable {
    length: number;
}

let s: string = "hello";
let x: Lengthable = s;
x;

// expect: hello