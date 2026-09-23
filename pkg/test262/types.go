// Package test262 holds result types shared between the Test262 runner
// (`cmd/paserati-test262`) and the failure-clustering tool
// (`cmd/paserati-analyze`).
package test262

import "time"

// Policy versions how results are interpreted. Results (and -dump
// baselines) from different policies are not comparable: policy 2 checks
// negative phase and type, async completion and the strict variant, which
// policy 1 did not.
const Policy = 2

// Status is the terminal outcome of one execution variant or one file.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
	StatusSkip    Status = "skip"
	// StatusInfra is a failure of the runner itself (unreadable test or
	// include, a worker that would not stop), not of the JavaScript.
	StatusInfra Status = "infra-error"
)

// Stats tracks aggregate statistics across a Test262 run. The file counts
// sum to Total; the Variant counts are over individual executions.
type Stats struct {
	Total          int
	Passed         int
	Failed         int
	Timeouts       int
	Skipped        int
	InfraErrors    int
	Variants       int
	VariantsPassed int
	Duration       time.Duration
}

// VariantResult is one execution of a test file: sloppy, strict, module
// or raw. Key is "<path relative to test/>#<variant>".
type VariantResult struct {
	Key     string `json:"key"`
	Variant string `json:"variant"`
	Status  Status `json:"status"`
	// Phase and ErrorType describe the error observed, if any: phase is
	// parse, resolution or runtime; ErrorType the thrown constructor's name.
	Phase      string        `json:"phase,omitempty"`
	ErrorType  string        `json:"errorType,omitempty"`
	Async      string        `json:"async,omitempty"`
	Diagnostic string        `json:"diagnostic,omitempty"`
	Duration   time.Duration `json:"duration"`
}

// Result is the per-file outcome serialized in `-json` mode. A file passes
// only if every variant it requires passes.
type Result struct {
	Path     string          `json:"path"`
	Status   Status          `json:"status"`
	Passed   bool            `json:"passed"`
	Failed   bool            `json:"failed"`
	TimedOut bool            `json:"timedOut"`
	Skipped  bool            `json:"skipped"`
	Infra    bool            `json:"infra"`
	Duration time.Duration   `json:"duration"`
	Error    string          `json:"error,omitempty"`
	Variants []VariantResult `json:"variants,omitempty"`
}

// Output is the top-level JSON envelope written by paserati-test262 -json.
type Output struct {
	Policy  int      `json:"policy"`
	Corpus  string   `json:"corpus,omitempty"`
	Stats   Stats    `json:"stats"`
	Results []Result `json:"results"`
}
