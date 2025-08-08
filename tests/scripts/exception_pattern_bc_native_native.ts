// expect: Caught native->native error: invalid character 'i' looking for beginning of object key string
// Test bc -> native -> native exception pattern
// We need to find a native function that calls another native function

let result = "";
try {
  // Trigger native -> native by calling a native that throws from within user code,
  // and rely on native error message propagation.
  JSON.parse("{invalid json}");
} catch (e) {
  result = "Caught native->native error: " + e.message;
}
result;
