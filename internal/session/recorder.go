package session

import (
	"errors"
	"sync"
	"time"
)

// Writer is the ordered, append-only transcript sink a Recorder drives. *Log
// is the production implementation (opened with Open); tests inject blocking
// or failing fakes through NewRecorderWithOpener so the recorder's guarantees
// — never block the caller, hold append order, fail exactly once — are proven
// without touching a real filesystem.
type Writer interface {
	Append(role, model, content, meta string, at time.Time) error
	Flush() error
	Close() error
	Path() string
}

// OpenWriter opens the per-run transcript sink under dir (a *Log in
// production). It is the injectable filesystem seam of the recorder: Open is
// called lazily by the worker on the first append/flush job, never by a UI
// update.
type OpenWriter func(dir, host string) (Writer, error)

// Result is the outcome of one ordered recorder job. Path is the transcript
// file for a flush ("" when nothing was recorded yet); Err is nil on success.
// Every enqueued job acks exactly one Result, so a waiter can never hang.
type Result struct {
	Path string
	Err  error
}

// ErrRecorderBacklog is returned by Append/Flush when the recorder's bounded
// queue is full — the sink is wedged and recording has already failed. It is
// a stable error the UI surfaces once; it never blocks an update.
var ErrRecorderBacklog = errors.New("transcript backlog full — recording stopped")

// recorderQueueCap bounds how many committed turns may wait for a stalled
// sink before new enqueues fail fast. Commits are human-paced (one per user
// message plus one per assistant reply), so 256 pending turns is far beyond
// any real backlog; the cap only exists so a wedged writer cannot make an
// enqueue (and with it the UI update that triggers it) block.
const recorderQueueCap = 256

// Recorder is the M-04 boundary: a single ordered background worker that owns
// the transcript Log, so no transcript filesystem work ever runs on the Bubble
// Tea update loop. The UI enqueues immutable append/flush jobs and reads back
// per-job Result acks; it never calls Open/Append/Flush/Close directly. One
// worker owns the Log, so transcript order is exactly the order the jobs were
// accepted (channel FIFO) — deterministic across user and assistant turns.
//
// Failure discipline: the first write failure (open, append, or flush)
// disables recording. Every later job acks the same stable failure so no
// waiter is stranded, the broken writer is closed, and Close still terminates
// the worker. No further I/O is attempted after the first failure.
type Recorder struct {
	dir  string
	host string
	open OpenWriter

	jobs chan job
	done chan struct{} // closed when the worker exits

	closeOnce sync.Once
	closeErr  error
}

type jobKind int

const (
	jobAppend jobKind = iota
	jobFlush
	jobClose
)

type job struct {
	kind jobKind
	// append payload (immutable)
	role, model, content, meta string
	at                         time.Time
	ack                        chan Result
}

// NewRecorder starts an ordered background transcript writer for dir/host.
// The transcript file is opened lazily by the worker on the first recorded
// turn (mirroring the previous lazy Open semantics), never by the caller.
func NewRecorder(dir, host string) *Recorder {
	return NewRecorderWithOpener(dir, host, func(d, h string) (Writer, error) {
		return Open(d, h)
	})
}

// NewRecorderWithOpener is NewRecorder with the filesystem seam injected: the
// worker opens its sink through open instead of session.Open. Production call
// ers use NewRecorder; tests use this to stall or fail the sink deterministically.
func NewRecorderWithOpener(dir, host string, open OpenWriter) *Recorder {
	r := &Recorder{
		dir:  dir,
		host: host,
		open: open,
		jobs: make(chan job, recorderQueueCap),
		done: make(chan struct{}),
	}
	go r.run()
	return r
}

// Append enqueues one committed turn for ordered background writing. It never
// blocks: it returns the job's ack channel (receives exactly one Result) and
// an error only when the backlog is full (recording has already failed).
func (r *Recorder) Append(role, model, content, meta string, at time.Time) (<-chan Result, error) {
	return r.enqueue(job{
		kind:    jobAppend,
		role:    role,
		model:   model,
		content: content,
		meta:    meta,
		at:      at,
	})
}

// Flush enqueues a flush of every earlier appended turn (the /export barrier):
// the job is processed strictly after all previously accepted appends, so the
// ack reports the exact transcript path only once those turns are durable.
// Flush never blocks the caller either.
func (r *Recorder) Flush() (<-chan Result, error) {
	return r.enqueue(job{kind: jobFlush})
}

// enqueue is the non-blocking producer path. The ack channel is buffered so a
// waiter that is created slightly after the worker acks (or never runs) can
// never block the worker.
func (r *Recorder) enqueue(j job) (<-chan Result, error) {
	j.ack = make(chan Result, 1)
	select {
	case r.jobs <- j:
		return j.ack, nil
	default:
		return nil, ErrRecorderBacklog
	}
}

// Close flushes every enqueued turn, closes the transcript, and stops the
// worker (normal-shutdown lifecycle). It blocks until the worker has exited so
// a process can rely on the file being complete and the goroutine gone; it is
// idempotent. On a wedged sink the durable close waits, matching the
// append-only contract — process exit remains the escape hatch.
func (r *Recorder) Close() error {
	r.closeOnce.Do(func() {
		ack := make(chan Result, 1)
		// A blocking enqueue is correct here: shutdown happens once, and the
		// close job must be accepted even if the queue is momentarily full.
		r.jobs <- job{kind: jobClose, ack: ack}
		res := <-ack
		r.closeErr = res.Err
		<-r.done // the worker has exited; nothing is left running
	})
	return r.closeErr
}

// run is the single worker. It owns the writer exclusively: Open, Append,
// Flush, and Close all happen here, serially, in job-acceptance order.
func (r *Recorder) run() {
	defer close(r.done)

	var w Writer
	var failErr error // first failure; stable for every later job

	openFor := func(j job) (Writer, error) {
		if failErr != nil {
			return nil, failErr
		}
		nw, err := r.open(r.dir, r.host)
		if err != nil {
			failErr = err
			return nil, failErr
		}
		return nw, nil
	}

	for j := range r.jobs {
		switch j.kind {
		case jobAppend:
			if w == nil {
				if nw, err := openFor(j); err != nil {
					j.ack <- Result{Err: err}
					continue
				} else {
					w = nw
				}
			}
			if err := w.Append(j.role, j.model, j.content, j.meta, j.at); err != nil {
				failErr = err
				w.Close() // best effort; the sink is broken, drop it
				w = nil
				j.ack <- Result{Err: err}
				continue
			}
			j.ack <- Result{}

		case jobFlush:
			res := Result{}
			switch {
			case failErr != nil:
				res.Err = failErr
			case w != nil:
				res.Path = w.Path()
				if err := w.Flush(); err != nil {
					failErr = err
					res.Err = err
					w.Close()
					w = nil
				}
			}
			// w == nil with no failure: nothing was recorded yet, so the
			// export reports no path and no error (the UI says so).
			j.ack <- res

		case jobClose:
			res := Result{}
			if w != nil {
				res.Err = w.Close()
			}
			j.ack <- res
			return
		}
	}
}
