package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// V2e unit tests (docs/v2e-multifile-undo-design.md §5): batch validation
// (caps, per-op authorize/contain), TOCTOU reject-at-apply, mid-apply
// rollback leaves the tree exactly at pre-state (fault-injected), journal
// write-ahead ordering, undo/redo round-trips, refuse-guards on external
// change, bounds/eviction, and crash-recovery detection.

func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func denyAll(path string) error { return fmt.Errorf("deny %s", path) }

func TestValidateWriteFilesArgs(t *testing.T) {
	op := func(path, kind, content, old, new string) writeFilesOp {
		return writeFilesOp{Path: path, Kind: kind, Content: content, Old: old, New: new}
	}
	ok := writeFilesArgs{Ops: []writeFilesOp{
		op("a.go", "create", "pkg a", "", ""),
		op("b.go", "edit", "", "old", "new"),
	}}
	if err := validateWriteFilesArgs(ok); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}
	cases := []struct {
		name string
		args writeFilesArgs
		want string
	}{
		{"empty ops", writeFilesArgs{}, "at least one op"},
		{"too many ops", writeFilesArgs{Ops: make([]writeFilesOp, maxWriteFilesOps+1)}, "exceeds the 16-op"},
		{"missing path", writeFilesArgs{Ops: []writeFilesOp{op("", "create", "x", "", "")}}, "path is required"},
		{"bad kind", writeFilesArgs{Ops: []writeFilesOp{op("a", "delete", "", "", "")}}, "unknown kind"},
		{"oversized content", writeFilesArgs{Ops: []writeFilesOp{op("a", "create", strings.Repeat("x", maxWriteFilesOpBytes+1), "", "")}}, "content exceeds"},
		{"oversized old", writeFilesArgs{Ops: []writeFilesOp{op("a", "edit", "", strings.Repeat("o", maxWriteFilesOpBytes+1), "n")}}, "old exceeds"},
		{"edit with oversized content", writeFilesArgs{Ops: []writeFilesOp{op("a", "edit", strings.Repeat("x", maxWriteFilesOpBytes+1), "o", "n")}}, "content exceeds"},
		{"create with oversized old", writeFilesArgs{Ops: []writeFilesOp{op("a", "create", "c", strings.Repeat("o", maxWriteFilesOpBytes+1), "")}}, "old exceeds"},
		{"edit missing old", writeFilesArgs{Ops: []writeFilesOp{op("a", "edit", "", "", "n")}}, "old is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWriteFilesArgs(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// flakyWrite is a fault-injecting writer: it fails the nth call (count is
// 1-based), letting the caller script mid-apply failures deterministically.
// With allFailAfter set, every call at or past failAt fails too (the shape
// for rollback-failure tests).
type flakyWrite struct {
	failAt       int64
	allFailAfter bool
	calls        atomic.Int64
	lastWrite    atomic.Value // string path of the last successful write
}

func (f *flakyWrite) Write(path string, content []byte, mode os.FileMode, tool, requested string) error {
	n := f.calls.Add(1)
	f.lastWrite.Store(path)
	if n == f.failAt || (f.allFailAfter && n > f.failAt) {
		return fmt.Errorf("injected disk failure at call %d (%s)", n, path)
	}
	return atomicWrite(path, content, mode, tool, requested)
}

func TestApplyBatchCreateAndEditRoundTrip(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "old.txt", "line one\nold text\nline three\n")
	journal, err := NewUndoJournal("")
	if err != nil {
		t.Fatal(err)
	}
	ops := []mutationOp{
		{kind: "create", path: "new.txt", content: []byte("hello\n"), verb: "created"},
		{kind: "edit", path: "old.txt", old: []byte("old text"), new: []byte("new text"), verb: "edited"},
	}
	lines, err := applyMutationSet(context.Background(), root, ops, nil, journal, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(lines) != 2 || lines[0] != "created new.txt" || lines[1] != "edited old.txt" {
		t.Fatalf("lines = %v", lines)
	}
	if got := readFile(t, root, "new.txt"); got != "hello\n" {
		t.Errorf("new.txt = %q", got)
	}
	if got := readFile(t, root, "old.txt"); got != "line one\nnew text\nline three\n" {
		t.Errorf("old.txt = %q", got)
	}
	if u, r := journal.Counts(); u != 1 || r != 0 {
		t.Errorf("journal counts = %d/%d, want 1 undo", u, r)
	}
	// The batch is one journal entry: one undo restores both files.
	res, err := journal.Undo()
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if res.Action != "undo" || len(res.Files) != 2 {
		t.Fatalf("undo result = %+v", res)
	}
	if exists(root, "new.txt") {
		t.Error("created file still exists after undo")
	}
	if got := readFile(t, root, "old.txt"); got != "line one\nold text\nline three\n" {
		t.Errorf("old.txt after undo = %q", got)
	}
}

func TestApplyBatchValidateAllAtApplyRejectsTOCTOU(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "target.txt", "original\n")
	// The edit op's old text exists at proposal time but is gone by apply:
	// validate-all-at-apply must reject the whole batch with a per-op reason.
	ops := []mutationOp{
		{kind: "create", path: "a.txt", content: []byte("x"), verb: "created"},
		{kind: "edit", path: "target.txt", old: []byte("proposal-time"), new: []byte("never"), verb: "edited"},
	}
	// Simulate the TOCTOU: remove target.txt before apply (an external edit).
	if err := os.Remove(filepath.Join(root, "target.txt")); err != nil {
		t.Fatal(err)
	}
	_, err := applyMutationSet(context.Background(), root, ops, nil, nil, nil)
	var bf batchFailure
	if !errors.As(err, &bf) || bf.opIndex != 2 || !strings.Contains(bf.reason, "no such file") {
		t.Fatalf("error = %v, want per-op rejection for op 2", err)
	}
	if exists(root, "a.txt") {
		t.Error("op 1 must not apply when op 2 fails validate-all (reject in full before apply)")
	}
}

func TestApplyBatchMidApplyFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "keep.txt", "keep me\n")
	mustWrite(t, root, "change.txt", "before\n")
	// Script: op1 create, op2 overwrite succeeds, op3 fails -> ops 1 and 2
	// must roll back exactly to pre-state (op1 removed, op2 restored).
	fw := &flakyWrite{failAt: 3}
	journal, _ := NewUndoJournal("")
	ops := []mutationOp{
		{kind: "create", path: "created.txt", content: []byte("c"), verb: "created"},
		{kind: "overwrite", path: "change.txt", content: []byte("after"), verb: "overwrote"},
		{kind: "overwrite", path: "keep.txt", content: []byte("nope"), verb: "overwrote"},
	}
	_, err := applyMutationSet(context.Background(), root, ops, nil, journal, fw.Write)
	var bf batchFailure
	if !errors.As(err, &bf) || bf.opIndex != 3 || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("error = %v, want op-3 injected failure", err)
	}
	if exists(root, "created.txt") {
		t.Error("op 1 created file not rolled back")
	}
	if got := readFile(t, root, "change.txt"); got != "before\n" {
		t.Errorf("op 2 not restored to pre-image: %q", got)
	}
	if got := readFile(t, root, "keep.txt"); got != "keep me\n" {
		t.Errorf("op 3 target modified despite failure: %q", got)
	}
	// Clean rollback discards the journal entry.
	if u, _ := journal.Counts(); u != 0 {
		t.Errorf("journal undo count = %d, want 0 after clean rollback", u)
	}
}

func TestApplyBatchRollbackFailureRetainsEntryForUndoRetry(t *testing.T) {
	// Owner decision #8: if the compensating rollback itself fails, the tree
	// and the journal entry stay untouched and the failure is loud naming the
	// files; a later /undo retries the restore.
	root := t.TempDir()
	mustWrite(t, root, "one.txt", "one-before\n")
	mustWrite(t, root, "two.txt", "two-before\n")
	// A writer that fails the SECOND call (the write of two.txt) and every
	// rollback write too (fail every call >= 2): apply op2 fails and the
	// rollback of op1 also fails.
	hard := &flakyWrite{failAt: 2}
	hard.allFailAfter = true
	journal, _ := NewUndoJournal("")
	ops := []mutationOp{
		{kind: "overwrite", path: "one.txt", content: []byte("one-after\n"), verb: "overwrote"},
		{kind: "overwrite", path: "two.txt", content: []byte("two-after\n"), verb: "overwrote"},
	}
	_, err := applyMutationSet(context.Background(), root, ops, nil, journal, hard.Write)
	if err == nil || !strings.Contains(err.Error(), "additionally rollback failed") {
		t.Fatalf("error = %v, want loud rollback failure", err)
	}
	// The tree is partially applied (one.txt changed, two.txt not) and the
	// entry is retained for a later /undo retry.
	if got := readFile(t, root, "one.txt"); got != "one-after\n" {
		t.Errorf("one.txt after failed rollback = %q, want partial state retained", got)
	}
	if u, _ := journal.Counts(); u != 1 {
		t.Fatalf("journal undo count = %d, want the partial entry retained", u)
	}
	// A later /undo (injection cleared) retries the full restore.
	res, err := journal.Undo()
	if err != nil {
		t.Fatalf("undo retry: %v", err)
	}
	if res.Action != "undo retry" {
		t.Errorf("undo retry action = %q", res.Action)
	}
	if got := readFile(t, root, "one.txt"); got != "one-before\n" {
		t.Errorf("one.txt after retry = %q", got)
	}
	if got := readFile(t, root, "two.txt"); got != "two-before\n" {
		t.Errorf("two.txt after retry = %q", got)
	}
	if u, _ := journal.Counts(); u != 0 {
		t.Errorf("journal count after successful retry = %d, want 0", u)
	}
}

func TestJournalWriteAheadBeforeFirstWrite(t *testing.T) {
	// The journal's Prepare fsyncs pre-image blobs to disk synchronously and
	// must return before the engine's first write can happen (design §4.2
	// step 5). Drive Prepare directly with a deterministic pre-image and a
	// subsequent failing write: the blobs must already be on disk at the
	// moment the write would have happened.
	root := t.TempDir()
	dir := t.TempDir()
	journal, err := NewUndoJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("aaa\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []FileRecord{{
		Requested: "a.txt",
		Path:      path,
		Mode:      0o600,
		Existed:   true,
		Pre:       []byte("aaa\n"),
		Post:      []byte("bbb\n"),
	}}
	entry, err := journal.Prepare(files)
	if err != nil {
		t.Fatal(err)
	}
	// Prepare returned: the fsynced pre-image must already be on disk.
	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatal(rerr)
	}
	found := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "entry-") {
			found++
			blobs, _ := os.ReadDir(filepath.Join(dir, e.Name()))
			if len(blobs) == 0 {
				t.Errorf("entry %s has no pre-image blob (write-ahead missing)", e.Name())
			}
		}
	}
	if found != 1 {
		t.Fatalf("journal entries on disk = %d, want 1 (capture must precede the first write)", found)
	}
	journal.Discard(entry)
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("discard did not remove the artifact: %d entries", len(entries))
	}
}

func TestUndoRedoRoundTripAndRefuseGuards(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "file.txt", "v1\n")
	journal, _ := NewUndoJournal("")
	ops := []mutationOp{{kind: "overwrite", path: "file.txt", content: []byte("v2\n"), verb: "overwrote"}}
	if _, err := applyMutationSet(context.Background(), root, ops, nil, journal, nil); err != nil {
		t.Fatal(err)
	}
	// Undo restores v1; redo restores v2.
	if _, err := journal.Undo(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "file.txt"); got != "v1\n" {
		t.Fatalf("after undo = %q", got)
	}
	if _, err := journal.Redo(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "file.txt"); got != "v2\n" {
		t.Fatalf("after redo = %q", got)
	}

	// External change after the mutation: undo must refuse the whole op,
	// naming the file, and leave both the file and the stack untouched.
	mustWrite(t, root, "file.txt", "user edit\n")
	_, err := journal.Undo()
	if err == nil || !strings.Contains(err.Error(), "undo refused") || !strings.Contains(err.Error(), "file.txt") {
		t.Fatalf("undo after external change = %v, want refusal naming file.txt", err)
	}
	if got := readFile(t, root, "file.txt"); got != "user edit\n" {
		t.Errorf("refused undo modified the file: %q", got)
	}
	if u, _ := journal.Counts(); u != 1 {
		t.Errorf("undo count = %d, want 1 (refused undo must not pop)", u)
	}
}

func TestUndoRefusedAfterCommandLikeWriteRetainsEntry(t *testing.T) {
	// Design §7 residual: undo after an intervening run_command that touched
	// the same files. Commands are never journaled (V2c); the refusal must
	// be a visible refusal naming the file (never a silent clobber), hint at
	// the run_command cause, and retain the entry so a later /undo can retry.
	root := t.TempDir()
	mustWrite(t, root, "f.txt", "v1\n")
	journal, _ := NewUndoJournal("")
	ops := []mutationOp{{kind: "overwrite", path: "f.txt", content: []byte("v2\n"), verb: "overwrote"}}
	if _, err := applyMutationSet(context.Background(), root, ops, nil, journal, nil); err != nil {
		t.Fatal(err)
	}
	// An approved run_command writes over f.txt after the mutation was
	// journaled (command effects are outside the journal).
	mustWrite(t, root, "f.txt", "command output\n")
	_, err := journal.Undo()
	if err == nil {
		t.Fatal("undo after a command write must be refused")
	}
	if !strings.Contains(err.Error(), "undo refused") || !strings.Contains(err.Error(), "f.txt") {
		t.Fatalf("refusal must name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "run_command") {
		t.Errorf("refusal should hint the run_command cause: %v", err)
	}
	// The file keeps the command's output and the entry stays for a retry.
	if got := readFile(t, root, "f.txt"); got != "command output\n" {
		t.Errorf("refused undo modified the command's output: %q", got)
	}
	if u, _ := journal.Counts(); u != 1 {
		t.Errorf("undo count = %d, want 1 (refused undo must not pop)", u)
	}
}

func TestJournalRedoRefusedWhenFileChangedAfterUndo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "f.txt", "one\n")
	journal, _ := NewUndoJournal("")
	ops := []mutationOp{{kind: "overwrite", path: "f.txt", content: []byte("two\n"), verb: "overwrote"}}
	if _, err := applyMutationSet(context.Background(), root, ops, nil, journal, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Undo(); err != nil {
		t.Fatal(err)
	}
	// A user edit lands after the undo: redo must refuse.
	mustWrite(t, root, "f.txt", "user after undo\n")
	if _, err := journal.Redo(); err == nil || !strings.Contains(err.Error(), "redo refused") {
		t.Fatalf("redo after external change = %v, want refusal", err)
	}
}

func TestJournalBoundsAndEviction(t *testing.T) {
	root := t.TempDir()
	journal, _ := NewUndoJournal("")
	for i := 0; i < maxJournalEntries+3; i++ {
		name := fmt.Sprintf("f%d.txt", i)
		mustWrite(t, root, name, "x")
		ops := []mutationOp{{kind: "overwrite", path: name, content: []byte("y"), verb: "overwrote"}}
		if _, err := applyMutationSet(context.Background(), root, ops, nil, journal, nil); err != nil {
			t.Fatal(err)
		}
	}
	if u, _ := journal.Counts(); u > maxJournalEntries {
		t.Errorf("undo entries = %d, cap is %d", u, maxJournalEntries)
	}
	// The oldest entries were evicted: undoing everything leaves the newest
	// maxJournalEntries reverted, and nothing errors.
	for u, _ := journal.Counts(); u > 0; u, _ = journal.Counts() {
		if _, err := journal.Undo(); err != nil {
			t.Fatalf("undo: %v", err)
		}
	}
	if u, _ := journal.Counts(); u != 0 {
		t.Errorf("undo count after drain = %d", u)
	}
}

func TestJournalCrashArtifactsDetectedAsStale(t *testing.T) {
	dir := t.TempDir()
	j, err := NewUndoJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWrite(t, root, "crash.txt", "before\n")
	// Prepare an entry but never commit/discard: simulates a crash mid-apply
	// (the process died between Prepare and Commit).
	files := []FileRecord{{
		Requested: "crash.txt",
		Path:      filepath.Join(root, "crash.txt"),
		Mode:      0o600,
		Existed:   true,
		Pre:       []byte("before\n"),
		Post:      []byte("after\n"),
	}}
	if _, err := j.Prepare(files); err != nil {
		t.Fatal(err)
	}
	// A clean exit removes artifacts; a crash leaves them. Simulate the
	// crash by abandoning j without Close.
	j2, err := NewUndoJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale := j2.Stale()
	if len(stale) != 1 {
		t.Fatalf("stale entries = %d, want 1", len(stale))
	}
	if len(stale[0].Files) != 1 || stale[0].Files[0] != "crash.txt" {
		t.Errorf("stale files = %v", stale[0].Files)
	}
	// Close GCs the artifacts (clean exit of the next run).
	if err := j2.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("artifacts not GC'd at clean close: %d entries", len(entries))
	}
}

func TestMutationRejectsGitPaths(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".git/hooks/pre-commit", "#!/bin/sh\n")
	// Single-file write into .git must be refused by the policy before any
	// dialog (design §2 gap row).
	err := (ToolPolicy{}).AuthorizePath(".git/hooks/pre-commit")
	if err == nil || !strings.Contains(err.Error(), ".git") {
		t.Fatalf("AuthorizePath(.git/hooks/pre-commit) = %v, want .git refusal", err)
	}
	// Direct engine call with the .git gate must refuse too.
	ops := []mutationOp{{kind: "overwrite", path: ".git/hooks/pre-commit", content: []byte("x"), verb: "overwrote"}}
	if _, err := applyMutationSet(context.Background(), root, ops, func(p string) error { return (ToolPolicy{}).AuthorizePath(p) }, nil, nil); err == nil {
		t.Fatal("batch write into .git succeeded")
	}
	// Ordinary files are unaffected.
	mustWrite(t, root, ".gitignore", "ignored\n")
	if err := (ToolPolicy{}).AuthorizePath(".gitignore"); err != nil {
		t.Fatalf("AuthorizePath(.gitignore) = %v, want allowed", err)
	}
}

// TestJournalRedoClearAccounting pins the reviewer P1 fix: Commit/KeepPartial
// must retire cleared redo entries out of the byte accounting AND remove
// their disk artifacts, or j.bytes drifts upward and LRU evicts live undo
// entries prematurely.
func TestJournalRedoClearAccounting(t *testing.T) {
	dir := t.TempDir()
	j, err := NewUndoJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mkRec := func(name, pre, post string) FileRecord {
		mustWrite(t, root, name, post)
		return FileRecord{
			Requested: name,
			Path:      filepath.Join(root, name),
			Mode:      0o600,
			Existed:   true,
			Pre:       []byte(pre),
			Post:      []byte(post),
		}
	}
	recA := mkRec("a.txt", "a\n", "aa\n")
	eA, err := j.Prepare([]FileRecord{recA})
	if err != nil {
		t.Fatal(err)
	}
	j.Commit(eA)
	if j.bytes != entryBytes(eA) {
		t.Fatalf("bytes after commit A = %d, want %d", j.bytes, entryBytes(eA))
	}
	// Undo moves A to the redo stack (bytes stay counted; disk stays).
	if _, err := j.Undo(); err != nil {
		t.Fatal(err)
	}
	// A new mutation commits B: the redo stack (A) is invalidated — its bytes
	// must leave the accounting and its disk dir must be removed.
	recB := mkRec("b.txt", "b\n", "bb\n")
	eB, err := j.Prepare([]FileRecord{recB})
	if err != nil {
		t.Fatal(err)
	}
	j.Commit(eB)
	if want := entryBytes(eB); j.bytes != want {
		t.Errorf("bytes after redo clear = %d, want %d (cleared redo must leave accounting)", j.bytes, want)
	}
	if undo, redo := j.Counts(); undo != 1 || redo != 0 {
		t.Errorf("counts = (%d, %d), want (1, 0)", undo, redo)
	}
	if edir := j.entryDirLocked(eA.id); edir != "" {
		t.Errorf("cleared redo entry A disk dir still present: %s", edir)
	}
	if edir := j.entryDirLocked(eB.id); edir == "" {
		t.Error("live entry B disk dir missing")
	}
	// KeepPartial clears redo the same way.
	eC, err := j.Prepare([]FileRecord{mkRec("c.txt", "c\n", "cc\n")})
	if err != nil {
		t.Fatal(err)
	}
	j.KeepPartial(eC)
	if want := entryBytes(eB) + entryBytes(eC); j.bytes != want {
		t.Errorf("bytes after KeepPartial = %d, want %d", j.bytes, want)
	}
}

// TestJournalDiskMetaContract pins the design §4.3 entry contract on disk:
// meta.json carries per-file {requested, path, mode, existed} at Prepare
// (write-ahead, post-hash empty) and the post-apply content hash after Commit.
func TestJournalDiskMetaContract(t *testing.T) {
	dir := t.TempDir()
	j, err := NewUndoJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWrite(t, root, "m.txt", "pre\n")
	rec := FileRecord{
		Requested: "m.txt",
		Path:      filepath.Join(root, "m.txt"),
		Mode:      0o640,
		Existed:   true,
		Pre:       []byte("pre\n"),
		Post:      []byte("post\n"),
	}
	e, err := j.Prepare([]FileRecord{rec})
	if err != nil {
		t.Fatal(err)
	}
	readMeta := func() undoMeta {
		t.Helper()
		edir := j.entryDirLocked(e.id)
		if edir == "" {
			t.Fatal("entry dir not found")
		}
		raw, err := os.ReadFile(filepath.Join(edir, "meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		var meta undoMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		return meta
	}
	meta := readMeta()
	if meta.Seq != e.id || len(meta.Files) != 1 {
		t.Fatalf("meta = %+v", meta)
	}
	f := meta.Files[0]
	if f.Requested != "m.txt" || f.Path != rec.Path || f.Mode != uint32(0o640) || !f.Existed {
		t.Errorf("meta file record = %+v, want requested/path/mode/existed of the mutation", f)
	}
	if f.PostHash != "" {
		t.Errorf("postHash set at Prepare time = %q, want empty (post does not exist yet)", f.PostHash)
	}
	j.Commit(e)
	meta = readMeta()
	sum := sha256.Sum256([]byte("post\n"))
	if got := meta.Files[0].PostHash; got != hex.EncodeToString(sum[:]) {
		t.Errorf("postHash after commit = %q, want %q", got, hex.EncodeToString(sum[:]))
	}
}

// (Prepare-failure artifact cleanup is defensive hygiene without a
// deterministic injection seam — Prepare has no writer hook to fault-inject
// through, and the failure mode is benign: a false stale report that Close
// GCs. Covered by inspection, not a contrived test.)
