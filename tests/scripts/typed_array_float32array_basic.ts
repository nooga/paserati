// expect: 3.1415927410125732
// Test Float32Array precision

const arr = new Float32Array(2);
arr[0] = 3.14159265359;
arr[1] = 1.0 / 3.0;

// Float32 has limited precision
arr[0];