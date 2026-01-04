// Test Paserati.reflect<T>() intrinsic
// expect: primitive-string

// Test basic primitive type reflection
const stringType = Paserati.reflect<string>();
stringType.kind + "-" + stringType.name;
