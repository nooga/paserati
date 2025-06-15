// Test rest elements in const declaration
const [head, ...tail] = ["a", "b", "c", "d"];
tail[2];
// expect: d