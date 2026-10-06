package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// Status of a job as seen by the parent.
const (
	StatusOK      = "ok"
	StatusTimeout = "timeout"
	StatusCrash   = "crash"
	StatusPanic   = "panic"
	StatusSkip    = "skip"
	StatusError   = "error" // harness-level failure (unreadable file)
)

// ---------------------------------------------------------------------------
// Worker side

// workerMain is the body of a `-worker` process. Jobs arrive as JSON lines on
// stdin; results leave as JSON lines on fd 3. Anything the checker prints to
// stdout is discarded so it cannot corrupt the protocol.
func workerMain(conformanceDir string) {
	// Results go to the real stdout; the os.Stdout variable is pointed away so
	// stray prints from the runtime under test cannot corrupt the JSON stream.
	// (Not an inherited extra fd: exec.Cmd.ExtraFiles is unsupported on Windows.)
	resp := os.Stdout
	if devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stdout = devnull
	}
	// A runaway recursion should die quickly (and be reported as a crash) rather
	// than grow towards Go's 1GB default stack limit.
	debug.SetMaxStack(256 << 20)

	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(resp)
	_ = enc.Encode(workerResp{Ready: true})
	n := 0
	for {
		var req workerReq
		if err := dec.Decode(&req); err != nil {
			return // parent closed stdin
		}
		// Every test starts from the same global state so results do not depend
		// on what ran before it in this worker.
		vm.ClearShapeCache()
		out := runJob(conformanceDir, req)
		if err := enc.Encode(&out); err != nil {
			return
		}
		n++
		if n%50 == 0 {
			runtime.GC()
		}
	}
}

// ---------------------------------------------------------------------------
// Parent side

// tailBuffer keeps the last few KB written to it (a worker's stderr).
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

const tailMax = 4096

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > tailMax {
		t.buf = t.buf[len(t.buf)-tailMax:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

func (t *tailBuffer) reset() {
	t.mu.Lock()
	t.buf = nil
	t.mu.Unlock()
}

// worker is one subprocess plus the plumbing to talk to it.
type worker struct {
	exe            string
	conformanceDir string
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	results        chan workerResp
	done           chan struct{} // closed when the result reader hits EOF
	stderr         *tailBuffer
	served         int
}

// recycleAfter bounds how long a worker lives so leaked memory cannot accumulate.
const recycleAfter = 300

func (w *worker) start() error {
	cmd := exec.Command(w.exe, "-worker", "-conformance-dir", w.conformanceDir)
	w.stderr = &tailBuffer{}
	cmd.Stderr = w.stderr
	resR, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	w.cmd = cmd
	w.stdin = stdin
	w.results = make(chan workerResp, 1)
	w.done = make(chan struct{})
	w.served = 0
	go func(results chan<- workerResp, done chan<- struct{}) {
		dec := json.NewDecoder(resR)
		for {
			var r workerResp
			if err := dec.Decode(&r); err != nil {
				break
			}
			results <- r
		}
		close(done)
	}(w.results, w.done)
	// Wait for the handshake so process startup (slow on loaded machines and on
	// Windows, where several binaries launching at once get scanned) is not
	// charged against the first job's deadline.
	select {
	case r := <-w.results:
		if r.Ready {
			return nil
		}
		w.stop(true)
		return fmt.Errorf("worker sent a result before its ready handshake")
	case <-w.done:
		w.stop(true)
		return fmt.Errorf("worker exited during startup: %s", lastLines(w.stderr.String()))
	case <-time.After(workerStartTimeout):
		w.stop(true)
		return fmt.Errorf("worker not ready after %v", workerStartTimeout)
	}
}

// workerStartTimeout bounds process startup, which is kept out of per-test deadlines.
const workerStartTimeout = 60 * time.Second

func (w *worker) stop(kill bool) {
	if w.cmd == nil {
		return
	}
	if kill {
		_ = w.cmd.Process.Kill()
	} else {
		_ = w.stdin.Close()
	}
	_ = w.cmd.Wait()
	<-w.done
	w.cmd = nil
}

// run sends one job and waits for its result under a hard deadline.
func (w *worker) run(req workerReq, deadline time.Duration) (workerResp, string, string) {
	if w.cmd != nil && w.served >= recycleAfter {
		w.stop(false)
	}
	if w.cmd == nil {
		if err := w.start(); err != nil {
			return workerResp{Err: "cannot start worker: " + err.Error()}, StatusError, ""
		}
	}
	w.stderr.reset()
	line, _ := json.Marshal(req)
	line = append(line, '\n')
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	if _, err := w.stdin.Write(line); err != nil {
		w.stop(true)
		return workerResp{}, StatusCrash, "worker died before job: " + lastLines(w.stderr.String())
	}
	w.served++
	select {
	case r := <-w.results:
		if r.Err != "" {
			return r, StatusError, r.Err
		}
		if r.Panic != "" {
			return r, StatusPanic, r.Panic
		}
		return r, StatusOK, ""
	case <-timer.C:
		w.stop(true)
		return workerResp{}, StatusTimeout, fmt.Sprintf("exceeded %v", deadline)
	case <-w.done:
		// The result pipe closed: either the worker crashed or it answered and
		// exited in the same instant; prefer a buffered answer.
		select {
		case r := <-w.results:
			return r, StatusOK, ""
		default:
		}
		w.stop(true)
		return workerResp{}, StatusCrash, lastLines(w.stderr.String())
	}
}

// lastLines trims a stderr tail to something printable on one line.
func lastLines(s string) string {
	lines := bytes.Split([]byte(s), []byte("\n"))
	var keep [][]byte
	for _, l := range lines {
		if len(bytes.TrimSpace(l)) > 0 {
			keep = append(keep, l)
		}
	}
	if len(keep) == 0 {
		return "worker exited without output"
	}
	// Go's fatal errors put the interesting line first ("fatal error: ...").
	for _, l := range keep {
		if bytes.HasPrefix(l, []byte("fatal error:")) || bytes.HasPrefix(l, []byte("runtime:")) || bytes.HasPrefix(l, []byte("panic:")) {
			return string(bytes.TrimSpace(l))
		}
	}
	return string(bytes.TrimSpace(keep[0]))
}

// jobOutcome is a worker response plus how the run ended.
type jobOutcome struct {
	Resp   workerResp
	Status string
	Detail string
}

// runPool executes jobs on n worker processes and returns outcomes indexed like
// jobs, so the result is independent of scheduling order and of n.
func runPool(exe, conformanceDir string, jobs []workerReq, n int, deadline time.Duration, progress func(done int)) []jobOutcome {
	out := make([]jobOutcome, len(jobs))
	if n < 1 {
		n = 1
	}
	if n > len(jobs) && len(jobs) > 0 {
		n = len(jobs)
	}
	idx := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := &worker{exe: exe, conformanceDir: conformanceDir}
			defer w.stop(false)
			for j := range idx {
				r, status, detail := w.run(jobs[j], deadline)
				out[j] = jobOutcome{Resp: r, Status: status, Detail: detail}
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				if progress != nil {
					progress(d)
				}
			}
		}()
	}
	for j := range jobs {
		idx <- j
	}
	close(idx)
	wg.Wait()
	return out
}
