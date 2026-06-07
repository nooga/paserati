// Package test262 holds result types shared between the Test262 runner
// (`cmd/paserati-test262`) and the failure-clustering tool
// (`cmd/paserati-analyze`).
package test262

import "time"

// Stats tracks aggregate statistics across a Test262 run.
type Stats struct {
	Total    int
	Passed   int
	Failed   int
	Timeouts int
	Skipped  int
	Duration time.Duration
}

// Result is the per-test outcome serialized in `-json` mode.
type Result struct {
	Path     string        `json:"path"`
	Passed   bool          `json:"passed"`
	Failed   bool          `json:"failed"`
	TimedOut bool          `json:"timedOut"`
	Skipped  bool          `json:"skipped"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// Output is the top-level JSON envelope written by paserati-test262 -json.
type Output struct {
	Stats   Stats    `json:"stats"`
	Results []Result `json:"results"`
}
