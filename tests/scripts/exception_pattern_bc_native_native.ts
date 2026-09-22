// expect: Caught native->native error: invalid character 'i' looking for beginning of value
// Test bc -> native -> native exception pattern
// We need to find a native function that calls another native function
//
// NOTE: this message comes verbatim from Go's encoding/json (see
// json_init.go's use of err.Error()), so it's tied to the exact Go version
// pinned in go.mod - it has changed wording across Go releases before. If
// this fails after a Go version bump, verify the new message with
// `go run ./cmd/paserati --no-typecheck -e '...'` under the new toolchain
// before assuming the test itself is stale.

let result = "";
try {
  // Trigger native -> native by calling a native that throws from within user code,
  // and rely on native error message propagation.
  JSON.parse("{invalid json}");
} catch (e) {
  result = "Caught native->native error: " + e.message;
}
result;
