// Package perfdata defines the JSON schema shared by the perf tools.
package perfdata

// Baseline is the on-disk format. Keep field tags stable.
type Baseline struct {
	Version       int                       `json:"version"`
	CapturedAt    string                    `json:"captured_at"`
	CapturedAtSHA string                    `json:"captured_at_sha"`
	Machine       Machine                   `json:"machine"`
	Anchor        Anchor                    `json:"anchor"`
	Benchmarks    map[string]BenchmarkEntry `json:"benchmarks"`
}

// Machine fingerprints the host that captured a baseline.
type Machine struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	NumCPU    int    `json:"num_cpu"`
	CPUModel  string `json:"cpu_model"`
	GoVersion string `json:"go_version"`
}

// Anchor captures the absolute speed of the calibration benchmark.
type Anchor struct {
	Name       string            `json:"name"`
	Package    string            `json:"package"`
	NSPerOp    float64           `json:"ns_per_op"`
	Iterations int64             `json:"iterations,omitempty"`
	Samples    []BenchmarkSample `json:"samples,omitempty"`
}

// BenchmarkEntry is one benchmark's current summary plus optional raw samples.
type BenchmarkEntry struct {
	NSPerOp       float64           `json:"ns_per_op"`
	AllocsPerOp   int64             `json:"allocs_per_op"`
	BytesPerOp    int64             `json:"bytes_per_op"`
	RatioToAnchor float64           `json:"ratio_to_anchor"`
	BestSinceSHA  string            `json:"best_since_sha,omitempty"`
	BestSinceAt   string            `json:"best_since_at,omitempty"`
	// SetHash fingerprints the set of operations aggregated into this measurement,
	// for records whose metric sums over a variable set (e.g. the test262 macro
	// sums per-test times over the passing, non-timed-out tests). Two captures
	// with equal counts but different sets get different hashes, so a consumer can
	// tell whether the number is comparable across snapshots. Empty for ordinary
	// single-op benchmarks, where the op is fixed and the count is the sample size.
	SetHash string            `json:"set_hash,omitempty"`
	Samples []BenchmarkSample `json:"samples,omitempty"`
}

// BenchmarkSample is one raw benchmark measurement retained for statistics.
type BenchmarkSample struct {
	Iterations    int64   `json:"iterations"`
	NSPerOp       float64 `json:"ns_per_op"`
	BytesPerOp    int64   `json:"bytes_per_op"`
	AllocsPerOp   int64   `json:"allocs_per_op"`
	RatioToAnchor float64 `json:"ratio_to_anchor,omitempty"`
	CapturedAt    string  `json:"captured_at,omitempty"`
}

// StreamRecord is one .jsonl line emitted by bench-ratchet capture.
type StreamRecord struct {
	Package     string  `json:"package"`
	Name        string  `json:"name"`
	Iterations  int64   `json:"iterations"`
	NSPerOp     float64 `json:"ns_per_op"`
	BytesPerOp  int64   `json:"bytes_per_op"`
	AllocsPerOp int64   `json:"allocs_per_op"`
	SetHash     string  `json:"set_hash,omitempty"` // identity of the aggregated set; see BenchmarkEntry.SetHash
	CapturedAt  string  `json:"captured_at"`
}

// Sample returns the record's benchmark measurement without identity fields.
func (r StreamRecord) Sample() BenchmarkSample {
	return BenchmarkSample{
		Iterations:  r.Iterations,
		NSPerOp:     r.NSPerOp,
		BytesPerOp:  r.BytesPerOp,
		AllocsPerOp: r.AllocsPerOp,
		CapturedAt:  r.CapturedAt,
	}
}
