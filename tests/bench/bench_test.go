package bench

import (
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"testing"
)

// compileFile compiles the given source file and handles errors.
// Uses testing.TB for compatibility with both tests and benchmarks.
func compileFile(tb testing.TB, filename string) *vm.Chunk {
	tb.Helper()
	chunk, compileErrs := driver.CompileFile(filename)
	if len(compileErrs) > 0 {
		var errMsgs []string
		for _, err := range compileErrs {
			errMsgs = append(errMsgs, err.Error())
		}
		tb.Fatalf("Compile errors in %q: %v", filename, errMsgs)
	}
	if chunk == nil {
		tb.Fatalf("Compilation succeeded for %q but returned nil chunk", filename)
	}
	return chunk
}

// BenchmarkFib runs the (currently placeholder) fib.ts script.
// Renamed to match BenchmarkXxx pattern.
func BenchmarkFibPlaceholderRun(b *testing.B) {
	// Compile once outside the loop.
	// Use the correct filename provided by the user.
	chunk := compileFile(b, "fib.ts")
	vmInstance := vm.NewVM()

	// Redirect stdout during benchmark to avoid polluting output
	// and potential overhead from printing.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	if err != nil {
		b.Fatalf("Failed to open os.DevNull: %v", err)
	}
	defer devNull.Close()
	oldStdout := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = oldStdout }() // Ensure stdout is restored

	b.ResetTimer() // Start timing after setup and compilation
	for i := 0; i < b.N; i++ {
		// Important: Reset the VM state for each iteration if Interpret doesn't fully reset
		// vmInstance.Reset() // Interpret() currently calls Reset(), so this might be redundant
		_ = vmInstance.Interpret(chunk) // Run the compiled code, ignore result for benchmark
	}
	b.StopTimer() // Stop timing

	// No need to restore stdout here due to defer
}
