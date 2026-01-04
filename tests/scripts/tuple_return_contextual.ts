// Test contextual typing for tuple return statements
// expect: hello

// Function returning tuple - array literal should be typed as tuple
function makeTuple(): [number, string, boolean] {
  return [42, "hello", true];  // Contextual typing from return type
}

let t = makeTuple();

// Access elements - should work with proper tuple typing
let str: string = t[1];
str;
