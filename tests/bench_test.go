package tests

import (
	"os"
	"paserati/pkg/builtins"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"strings"
	"testing"
)

// compileFile compiles the given source file and handles errors.
// Uses testing.TB for compatibility with both tests and benchmarks.
func compileFile(tb testing.TB, filename string) *vm.Chunk {
	tb.Helper()
	chunk, compileErrs := driver.CompileFile(filename) // Returns PaseratiError slice
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		for _, err := range compileErrs {
			errMsgs.WriteString(err.Error() + "\\n") // Use the error string directly
		}
		// Display formatted errors if possible (might require source)
		// errors.DisplayErrors(source, compileErrs) // Need source string here
		tb.Fatalf("Compile errors in %q:\\n%s", filename, errMsgs.String())
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
	chunk := compileFile(b, "scripts/factorial.ts")
	vmInstance := vm.NewVM()
	vmInstance.AddStandardCallbacks(builtins.GetStandardInitCallbacks())
	if err := vmInstance.InitializeWithCallbacks(); err != nil {
		b.Fatalf("VM initialization failed: %v", err)
	}

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

		// Run the compiled code, capture value and errors
		_, runtimeErrs := vmInstance.Interpret(chunk)

		// Fail benchmark immediately if runtime errors occur
		if len(runtimeErrs) > 0 {
			var errMsgs strings.Builder
			// Need source to display properly, just list errors for now
			for _, rErr := range runtimeErrs {
				errMsgs.WriteString(rErr.Error() + "\\n")
			}
			b.Fatalf("Runtime error during benchmark iteration %d:\\n%s", i, errMsgs.String())
		}
	}
	b.StopTimer() // Stop timing

	// No need to restore stdout here due to defer
}

// BenchmarkMatrixMult runs the matrix_mult.ts script.
func BenchmarkMatrixMult(b *testing.B) {
	// Compile once outside the loop.
	chunk := compileFile(b, "scripts/matrix_mult.ts")
	vmInstance := vm.NewVM()
	vmInstance.AddStandardCallbacks(builtins.GetStandardInitCallbacks())
	if err := vmInstance.InitializeWithCallbacks(); err != nil {
		b.Fatalf("VM initialization failed: %v", err)
	}

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
		// Interpret() currently calls Reset(), so no explicit reset needed here yet.

		// Run the compiled code, capture value and errors
		_, runtimeErrs := vmInstance.Interpret(chunk)

		// Fail benchmark immediately if runtime errors occur
		if len(runtimeErrs) > 0 {
			var errMsgs strings.Builder
			// Need source to display properly, just list errors for now
			for _, rErr := range runtimeErrs {
				errMsgs.WriteString(rErr.Error() + "\n")
			}
			b.Fatalf("Runtime error during benchmark iteration %d:\n%s", i, errMsgs.String())
		}
	}
	b.StopTimer() // Stop timing

}
