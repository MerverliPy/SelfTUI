package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// V2e write_files batch tool (docs/v2e-multifile-undo-design.md §4). The
// model proposes one coherent multi-file change as a single closed-schema
// call; the runner renders a per-file review, the user approves the whole
// batch or declines it, and only then is anything validated-and-applied
// (all-or-nothing, journaled for undo).
//
// Caps (hard proposal ceiling, rejected in full before any dialog): at most
// maxWriteFilesOps ops; per-op content/old/new ≤ maxWriteFilesOpBytes; the
// whole call already respects the existing 1 MiB per-call argument cap
// (maxToolArgBytes) enforced at the run boundary.

const (
	maxWriteFilesOps     = 16
	maxWriteFilesOpBytes = 256 << 10
	// maxReviewSourceBytes bounds how much current file content the review
	// overlay reads to render a before/after diff. Everything above the
	// single-file edit ceiling is display-truncated, never read wholesale:
	// the mutation itself never depends on the diff.
	maxReviewSourceBytes = maxReadBytes
)

// Batch review window (owner decision #2): 120 s default, 300 s hard cap.
// The batch stage replaces the generic 30/60 s mutation confirm because
// reading a real diff needs more than a glance; nothing is applied
// pre-approval, so an expiry stays safe (decline semantics, turn ends).
const (
	defaultBatchReviewTimeout = 120 * time.Second
	maxBatchReviewTimeout     = 300 * time.Second
)

// writeFilesOp is one element of the write_files ops array.
type writeFilesOp struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"` // "create" | "overwrite" | "edit"
	Content string `json:"content,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
}

// writeFilesArgs is the decoded write_files call payload.
type writeFilesArgs struct {
	Ops  []writeFilesOp `json:"ops"`
	Note string         `json:"note"`
}

// validateWriteFilesArgs rejects a batch in full before any dialog (the
// existing H-03 reject-in-full pattern): structural errors (missing path,
// unknown kind, missing kind-required fields) and per-op byte caps. The
// returned error names the first offending op.
func validateWriteFilesArgs(args writeFilesArgs) error {
	if len(args.Ops) == 0 {
		return errors.New("write_files: ops must contain at least one op")
	}
	if len(args.Ops) > maxWriteFilesOps {
		return fmt.Errorf("write_files: %d ops exceeds the %d-op batch cap", len(args.Ops), maxWriteFilesOps)
	}
	for i, op := range args.Ops {
		what := fmt.Sprintf("write_files op %d (%s)", i+1, displayOp(op))
		if op.Path == "" {
			return fmt.Errorf("%s: path is required", what)
		}
		// Per-op caps cover every schema field regardless of kind: all three
		// fields are schema-valid on any op, so an oversized unused field is
		// still payload the caller sent (review P1: kind-scoped checks let an
		// edit carry an oversized content and a create an oversized old/new).
		for _, field := range []struct {
			name string
			n    int
		}{{"content", len(op.Content)}, {"old", len(op.Old)}, {"new", len(op.New)}} {
			if field.n > maxWriteFilesOpBytes {
				return fmt.Errorf("%s: %s exceeds %d bytes", what, field.name, maxWriteFilesOpBytes)
			}
		}
		switch op.Kind {
		case "create", "overwrite":
		case "edit":
			if op.Old == "" {
				return fmt.Errorf("%s: old is required for an edit", what)
			}
		default:
			return fmt.Errorf("%s: unknown kind %q (create, overwrite, or edit)", what, op.Kind)
		}
	}
	return nil
}

func displayOp(op writeFilesOp) string {
	if op.Path == "" {
		return "<empty path>"
	}
	return op.Path
}

// BatchFileReview is the UI-facing summary of one file in a proposed batch:
// one summary row plus the rendered one-column diff rows (review overlay).
type BatchFileReview struct {
	Path    string
	Kind    string // create | overwrite | edit (verb for the summary row)
	Summary string // e.g. "M internal/agent/runner.go  +18 −6"
	Rows    []string
}

// BatchReviewMsg replaces the generic raw-args confirm for the write_files
// tool (design §4.2 step 1): the UI receives summary rows + rendered
// per-file unified diffs instead of a raw-args modal. Reply is one boolean
// for the whole batch — all-or-nothing; there is no per-file accept/reject
// in this cut. Decline or expiry apply nothing.
type BatchReviewMsg struct {
	Name      string
	Workspace string
	Timeout   time.Duration
	Files     []BatchFileReview
	Note      string
	reply     chan bool
}

// Respond releases the batch review with one whole-batch decision.
func (m BatchReviewMsg) Respond(approved bool) {
	select {
	case m.reply <- approved:
	default:
	}
}

// previewBatch reads the current tree and renders the per-file review for a
// proposed batch. Every read is jail-checked and size-bounded; a file that
// cannot be read or is too large to diff is summarized without a body. This
// is proposal-time display only — the authoritative validate-all happens at
// apply time against the then-current tree.
func (r *Runner) previewBatch(root string, args writeFilesArgs) ([]BatchFileReview, error) {
	files := make([]BatchFileReview, 0, len(args.Ops))
	for _, op := range args.Ops {
		review, err := r.previewOne(root, op)
		if err != nil {
			return nil, err
		}
		files = append(files, review)
	}
	return files, nil
}

// previewOne renders one op's review. Authorization runs at proposal (per-op,
// before any dialog) so a sensitive path can never reach the review stage. A
// pre-existing target over the per-file journal cap refuses the whole batch
// HERE, before the dialog (design §4.3: "larger ⇒ batch refused at proposal")
// — approving a batch whose pre-image cannot be journaled would only fail at
// apply.
func (r *Runner) previewOne(root string, op writeFilesOp) (BatchFileReview, error) {
	if err := r.authorizePath(op.Path); err != nil {
		return BatchFileReview{}, fmt.Errorf("write_files op for %q: %w", op.Path, err)
	}
	// Pre-image cap check needs only a stat, not a read: a pre-existing file
	// whose bytes the journal could not hold (over maxJournalFileBytes)
	// cannot be part of any journaled batch.
	if p, err := securePath(root, op.Path); err == nil {
		if info, serr := os.Stat(p); serr == nil && info.Mode().IsRegular() && info.Size() > maxJournalFileBytes {
			return BatchFileReview{}, fmt.Errorf("write_files op for %q: pre-image exceeds %d-byte journal cap", op.Path, maxJournalFileBytes)
		}
	}
	review := BatchFileReview{Path: op.Path, Kind: op.Kind}

	oldText, exists, err := r.readReviewSource(root, op.Path)
	if err != nil {
		// A file that cannot be read now is still reviewable as a change the
		// apply step will judge; summarize without a diff body.
		review.Summary = fmt.Sprintf("%s %s  (current state unavailable)", kindLetter(op.Kind), op.Path)
		return review, nil
	}
	if op.Kind == "create" {
		if exists {
			review.Summary = fmt.Sprintf("%s %s  (already exists — batch will fail at apply)", kindLetter(op.Kind), op.Path)
			return review, nil
		}
		rows, added, _ := lineDiff("", op.Content)
		review.Summary = fmt.Sprintf("%s %s  +%d", kindLetter(op.Kind), op.Path, added)
		review.Rows = renderDiffRows(rows)
		return review, nil
	}
	if !exists {
		// Overwrite of an absent file creates it (WriteFile overwrite=true
		// semantics); only an edit needs the target to exist already.
		if op.Kind == "edit" {
			review.Summary = fmt.Sprintf("%s %s  (missing — batch will fail at apply)", kindLetter(op.Kind), op.Path)
			return review, nil
		}
		rows, added, _ := lineDiff("", op.Content)
		review.Summary = fmt.Sprintf("%s %s  +%d", kindLetter(op.Kind), op.Path, added)
		review.Rows = renderDiffRows(rows)
		return review, nil
	}
	newText := op.Content
	if op.Kind == "edit" {
		if strings.Count(oldText, op.Old) != 1 {
			review.Summary = fmt.Sprintf("%s %s  (old text not found exactly once — batch will fail at apply)", kindLetter(op.Kind), op.Path)
			return review, nil
		}
		newText = strings.Replace(oldText, op.Old, op.New, 1)
	}
	rows, added, removed := lineDiff(oldText, newText)
	review.Summary = fmt.Sprintf("%s %s  +%d −%d", kindLetter(op.Kind), op.Path, added, removed)
	review.Rows = renderDiffRows(rows)
	return review, nil
}

// kindLetter is the git-style status letter of the summary row.
func kindLetter(kind string) string {
	switch kind {
	case "create":
		return "A"
	case "edit":
		return "M"
	default: // overwrite
		return "M"
	}
}

// readReviewSource reads a workspace file for the review overlay, bounded by
// maxReviewSourceBytes and jail-checked. exists=false when the file is absent
// (a create target, or an overwrite of a not-yet-existing file). Oversized or
// non-UTF-8 content is reported as existing-but-undeiffable so the summary
// row still names the file without reading it wholesale.
func (r *Runner) readReviewSource(root, requested string) (text string, exists bool, err error) {
	p, err := securePath(root, requested)
	if err != nil {
		// Not resolvable now (absent target, broken symlink, ...): treat as
		// absent; apply re-checks authoritatively.
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, nil
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, nil
	}
	if !info.Mode().IsRegular() {
		return "", true, nil // non-regular: summary-only row
	}
	if info.Size() > maxReviewSourceBytes {
		return "", true, nil // too large to diff: summary-only row
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", true, nil
	}
	if !utf8.Valid(b) {
		return "", true, nil // binary: summary-only row (no NUL-sniff refusal on the op itself)
	}
	return string(b), true, nil
}

// mutationOp is the apply engine's normalized per-file operation. kind and
// path mirror writeFilesOp; content/old/new are []byte versions; tool names
// the originating surface for error/result labeling (write_file, edit_file,
// or write_files).
type mutationOp struct {
	kind    string
	path    string
	content []byte
	old     []byte
	new     []byte
	verb    string // result verb: "created"/"overwrote"/"edited", or "wrote"
	tool    string // originating tool for error text: write_file|edit_file|write_files
}

func opFromWriteFilesOp(op writeFilesOp) mutationOp {
	m := mutationOp{kind: op.Kind, path: op.Path, content: []byte(op.Content), old: []byte(op.Old), new: []byte(op.New), tool: "write_files"}
	switch op.Kind {
	case "create":
		m.verb = "created"
	case "overwrite":
		m.verb = "overwrote"
	case "edit":
		m.verb = "edited"
	}
	return m
}

// mutationOpFromSingle maps a single-file tool call onto the same engine as a
// 1-op change-set (owner decision #3: all confirmed mutations journal, and
// single write_file/edit_file calls are 1-op change-sets).
func mutationOpFromSingle(tool string, path, content, old, new string, overwrite bool) mutationOp {
	switch tool {
	case "edit_file":
		return mutationOp{kind: "edit", path: path, content: []byte(content), old: []byte(old), new: []byte(new), verb: "edited", tool: "edit_file"}
	default: // write_file
		kind := "overwrite"
		if !overwrite {
			kind = "create"
		}
		return mutationOp{kind: kind, path: path, content: []byte(content), verb: "wrote", tool: "write_file"}
	}
}

// batchFailure is a whole-batch apply failure carrying the per-op reason the
// model should see (compact, one failure line per failing op).
type batchFailure struct {
	opIndex int // 1-based; 0 when the failure is batch-wide
	op      string
	reason  string
}

func (f batchFailure) Error() string {
	if f.opIndex > 0 {
		return fmt.Sprintf("write_files op %d (%s): %s", f.opIndex, f.op, f.reason)
	}
	return f.reason
}

// applyMutationSet is the all-or-nothing mutation engine shared by the
// single-file tools (1-op sets) and write_files (multi-op sets). Order:
//
//  1. validate every op against the CURRENT tree — authorize (incl. the
//     lexical .git refusal, which lives in AuthorizePath), containment, kind
//     rules, and (edit) exact-one-match; a TOCTOU race since proposal fails
//     the whole set with a per-op reason (design §4.2 step 4).
//  2. M-06 ctx gate.
//  3. journal write-ahead: capture pre-images and fsync the entry before the
//     first write (design §4.2 step 5). Pre-images are captured whenever a
//     rollback could need them (journaling, or a multi-op set); the per-file
//     pre-image cap is a journal bound, enforced only when journaling.
//  4. apply sequentially through the existing atomic primitives.
//  5. any mid-apply failure rolls back the already-applied files from the
//     pre-images (all-or-nothing; a created file is removed); the journal
//     entry is discarded. If the rollback itself fails the entry is retained
//     for a later /undo retry and the failure is loud (owner decision #8).
//
// When journal is nil (plain runners, tests without an undo store) the set
// still applies all-or-nothing, but nothing is recorded and there is nothing
// to undo — the caller's choice.
func applyMutationSet(ctx context.Context, root string, ops []mutationOp, authorize func(string) error, journal *UndoJournal, write func(path string, content []byte, mode os.FileMode, tool, requested string) error) ([]string, error) {
	if len(ops) == 0 {
		return nil, errors.New("write_files: no ops to apply")
	}
	if write == nil {
		write = atomicWrite
	}
	// Pre-images are only worth capturing when a rollback could restore from
	// them: journaled sets always keep an entry, and multi-op sets need them
	// if a later op fails. A single op without a journal is a single atomic
	// write — nothing to roll back, nothing to read.
	needPre := journal != nil || len(ops) > 1

	// Step 1: validate every op against the current tree.
	validated := make([]validatedOp, 0, len(ops))
	for i, op := range ops {
		v, err := validateMutationOp(root, op, authorize, needPre, journal != nil)
		if err != nil {
			if len(ops) == 1 {
				return nil, err
			}
			return nil, batchFailure{opIndex: i + 1, op: op.path, reason: err.Error()}
		}
		validated = append(validated, v)
	}

	// Step 2: M-06 gate.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Step 3: journal write-ahead (capture pre-images, fsync before the first
	// write).
	var entry *undoEntry
	if journal != nil {
		records := make([]FileRecord, 0, len(validated))
		for _, v := range validated {
			records = append(records, FileRecord{
				Requested: v.op.path,
				Path:      v.path,
				Mode:      v.mode,
				Existed:   v.existed,
				Pre:       v.pre,
				Post:      v.content,
			})
		}
		var err error
		entry, err = journal.Prepare(records)
		if err != nil {
			if len(ops) == 1 {
				return nil, err
			}
			return nil, batchFailure{reason: err.Error()}
		}
	}

	// Step 4+5: apply sequentially; roll back on any failure.
	lines := make([]string, 0, len(ops))
	for i, v := range validated {
		if err := ctx.Err(); err != nil {
			// Cancellation mid-apply = full rollback; the entry is discarded
			// (owner decision #5).
			rbErr := rollbackApplied(validated[:i], write)
			if rbErr != nil {
				// Rollback failed: retain the entry for a later /undo retry
				// and fail loudly naming the files (owner decision #8).
				if journal != nil && entry != nil {
					journal.KeepPartial(entry)
				}
				return nil, fmt.Errorf("%s; additionally rollback failed: %v", err, rbErr)
			}
			if journal != nil {
				journal.Discard(entry)
			}
			return nil, err
		}
		if err := applyOne(v, write); err != nil {
			rbErr := rollbackApplied(validated[:i], write)
			if rbErr != nil {
				if journal != nil && entry != nil {
					journal.KeepPartial(entry)
				}
				return nil, fmt.Errorf("%v; additionally rollback failed: %v", err, rbErr)
			}
			if journal != nil {
				journal.Discard(entry)
			}
			if len(ops) == 1 {
				return nil, err
			}
			return nil, batchFailure{opIndex: i + 1, op: v.op.path, reason: err.Error()}
		}
		lines = append(lines, v.op.verb+" "+v.op.path)
	}
	if journal != nil {
		journal.Commit(entry)
	}
	return lines, nil
}

// validatedOp is one op resolved and checked against the current tree.
type validatedOp struct {
	op      mutationOp
	path    string // canonical absolute write target
	mode    os.FileMode
	existed bool
	pre     []byte // pre-image (nil when !existed or pre not needed)
	content []byte // final content to write (edit ops already replaced)
}

// validateMutationOp checks one op against the current tree and computes its
// write target, mode, pre-image, and final content. The error text mirrors
// the single-file primitives so a 1-op set reads exactly like today's tools.
// needPre controls whether the pre-image is read at all (only worth it when a
// rollback or a journal entry could need it); enforceCap applies the 8 MiB
// per-file pre-image cap (a journal bound).
func validateMutationOp(root string, op mutationOp, authorize func(string) error, needPre, enforceCap bool) (validatedOp, error) {
	var out validatedOp
	out.op = op
	if authorize != nil {
		if err := authorize(op.path); err != nil {
			return out, err
		}
	}
	switch op.kind {
	case "create":
		path, err := secureWritePath(root, op.path)
		if err != nil {
			return out, wrapToolErr(op, err)
		}
		if info, serr := os.Lstat(path); serr == nil {
			if !info.Mode().IsRegular() {
				return out, wrapToolErr(op, fmt.Errorf("not a regular file"))
			}
			return out, wrapToolErr(op, errors.New("already exists (set overwrite to true)"))
		} else if !errors.Is(serr, os.ErrNotExist) {
			return out, wrapToolErr(op, serr)
		}
		out.path = path
		out.mode = 0o600
		out.existed = false
		out.content = op.content
		return out, nil

	case "overwrite":
		path, err := secureWritePath(root, op.path)
		if err != nil {
			return out, wrapToolErr(op, err)
		}
		out.path = path
		out.mode = 0o600
		if info, serr := os.Lstat(path); serr == nil {
			if !info.Mode().IsRegular() {
				return out, wrapToolErr(op, fmt.Errorf("not a regular file"))
			}
			out.mode = info.Mode().Perm()
			out.existed = true
		} else if !errors.Is(serr, os.ErrNotExist) {
			return out, wrapToolErr(op, serr)
		}
		out.content = op.content
		if out.existed && needPre {
			pre, err := readPreImage(path, op.path, enforceCap)
			if err != nil {
				return out, wrapToolErr(op, err)
			}
			out.pre = pre
		}
		return out, nil

	case "edit":
		path, err := securePath(root, op.path)
		if err != nil {
			return out, wrapToolErr(op, err)
		}
		out.path = path
		info, err := os.Stat(path)
		if err != nil {
			return out, wrapToolErr(op, err)
		}
		if !info.Mode().IsRegular() {
			return out, wrapToolErr(op, fmt.Errorf("not a regular file"))
		}
		if info.Size() > maxReadBytes {
			return out, wrapToolErr(op, fmt.Errorf("file exceeds %d-byte limit", maxReadBytes))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return out, wrapToolErr(op, err)
		}
		if strings.Count(string(body), string(op.old)) != 1 {
			return out, wrapToolErr(op, errors.New("exact old text not found exactly once"))
		}
		out.mode = info.Mode().Perm()
		out.existed = true
		if needPre {
			out.pre = body
		}
		out.content = []byte(strings.Replace(string(body), string(op.old), string(op.new), 1))
		return out, nil
	default:
		return out, fmt.Errorf("unknown mutation kind %q", op.kind)
	}
}

// readPreImage reads an existing file's bytes as the journal pre-image,
// refusing content over the per-file journal cap when enforced.
func readPreImage(path, requested string, enforceCap bool) ([]byte, error) {
	if enforceCap {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.Size() > maxJournalFileBytes {
			return nil, fmt.Errorf("pre-image for %s exceeds %d-byte journal cap", requested, maxJournalFileBytes)
		}
	}
	return os.ReadFile(path)
}

func wrapToolErr(op mutationOp, err error) error {
	tool := op.tool
	if tool == "" {
		tool = "write_files"
	}
	return fmt.Errorf("%s %q: %v", tool, op.path, err)
}

// applyOne performs the final atomic write of one validated op.
func applyOne(v validatedOp, write func(path string, content []byte, mode os.FileMode, tool, requested string) error) error {
	tool := v.op.tool
	if tool == "" {
		tool = "write_files"
	}
	return write(v.path, v.content, v.mode, tool, v.op.path)
}

// rollbackApplied restores the already-applied ops from their pre-images (or
// removes files a create op added). Returns an error only when the restore
// itself failed — the caller then retains the journal entry and reports loud.
func rollbackApplied(applied []validatedOp, write func(path string, content []byte, mode os.FileMode, tool, requested string) error) error {
	if write == nil {
		write = atomicWrite
	}
	var firstErr error
	for i := len(applied) - 1; i >= 0; i-- {
		v := applied[i]
		var err error
		if !v.existed {
			err = os.Remove(v.path)
			if err != nil && os.IsNotExist(err) {
				err = nil
			}
		} else if v.pre != nil {
			tool := v.op.tool
			if tool == "" {
				tool = "write_files"
			}
			err = write(v.path, v.pre, v.mode, tool, v.op.path)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("restore %s: %w", v.op.path, err)
		}
	}
	return firstErr
}
