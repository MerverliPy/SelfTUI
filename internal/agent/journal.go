package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UndoJournal is the session-scoped undo/redo store for confirmed agent
// mutations (V2e, docs/v2e-multifile-undo-design.md §4.3). Every confirmed
// single-file mutation and every applied batch is recorded here as one
// change-set entry (single-file calls are 1-op change-sets), so /undo pops
// the newest entry and /redo restores it.
//
// The journal is deliberately "in-process, never sandboxed": undo/redo shell
// nothing out; they are plain Go over the journal, host-side, because the
// sandboxed git is read-only by design. Entry stacks are session-scoped (they
// die with the process); the optional disk dir holds fsynced pre-image blobs
// as crash-recovery artifacts that are GC'd at clean exit (Close). A process
// crash mid-apply therefore leaves a detectable stale entry (Stale) that the
// next start reports without auto-reverting.
//
// Concurrency: the runner goroutine records entries while the UI goroutine
// runs /undo and /redo; every method takes the internal mutex, and the UI
// additionally refuses undo while a turn is streaming so a mutation can never
// race an undo of the same entry.
type UndoJournal struct {
	mu     sync.Mutex
	dir    string // empty = memory-only (no crash artifacts)
	undo   []*undoEntry
	redo   []*undoEntry
	bytes  int64 // retained pre+post image bytes across both stacks
	seq    int64
	stale  []StaleEntry // crash leftovers detected at open
	closed bool
}

// StaleEntry is one crash-left journal artifact found at open: a previous
// process died mid-apply, leaving fsynced pre-images. It is reported, never
// auto-reverted (design §4.3).
type StaleEntry struct {
	Dir   string   // artifact directory under the journal root
	Files []string // requested paths the interrupted entry touched (meta read)
}

// UndoJournal bounds (owner decision #4): at most 25 entries, 32 MiB of
// retained image bytes total, 8 MiB per-file pre-image (a larger pre-image
// refuses the mutation at capture). LRU eviction drops the oldest entries.
const (
	maxJournalEntries   = 25
	maxJournalBytes     = 32 << 20
	maxJournalFileBytes = 8 << 20
)

var (
	// ErrNothingToUndo / ErrNothingToRedo are the empty-stack results. /undo
	// and /redo surface them as notices ("nothing to undo").
	ErrNothingToUndo = errors.New("nothing to undo")
	ErrNothingToRedo = errors.New("nothing to redo")
)

// FileRecord is one file's mutation, handed to the journal by the apply
// engine with the pre-image already captured and the post (applied) content
// already computed but not yet written. The journal fsyncs the pre-image
// blob before the engine performs its first atomicWrite (capture-before-
// mutate ordering, design §4.2 step 5).
type FileRecord struct {
	Requested string      // workspace-relative path as the model wrote it
	Path      string      // canonical absolute path the mutation will write
	Mode      os.FileMode // pre-existing mode (or 0600 for a new file)
	Existed   bool        // the file existed before this mutation
	Pre       []byte      // nil when !Existed
	Post      []byte      // the content that will be written (never nil)
}

// undoEntry is one recorded change-set.
type undoEntry struct {
	id      int64
	files   []FileRecord
	partial bool // apply failed and rollback failed too: keep for a /undo retry
}

// NewUndoJournal opens the journal. dir is the crash-artifact root
// ($XDG_STATE_HOME/selftui/undo in production; empty keeps the journal
// memory-only). Any leftover entry directories are reported as Stale — a
// clean exit removes them all (Close), so anything found at open is a crash
// artifact and is never auto-reverted.
func NewUndoJournal(dir string) (*UndoJournal, error) {
	j := &UndoJournal{dir: dir}
	if dir == "" {
		return j, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("undo journal: create %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("undo journal: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "entry-") {
			continue
		}
		edir := filepath.Join(dir, e.Name())
		j.stale = append(j.stale, StaleEntry{Dir: edir, Files: readStaleFiles(edir)})
	}
	return j, nil
}

// readStaleFiles extracts the requested paths a crash-left entry touched,
// best effort: a torn meta file still leaves the entry detectable, just
// without file names.
func readStaleFiles(edir string) []string {
	raw, err := os.ReadFile(filepath.Join(edir, "meta.json"))
	if err != nil {
		return nil
	}
	var meta undoMeta
	if json.Unmarshal(raw, &meta) != nil {
		return nil
	}
	out := make([]string, 0, len(meta.Files))
	for _, f := range meta.Files {
		out = append(out, f.Requested)
	}
	return out
}

// Stale returns the crash-left entries found at open (nil when clean).
func (j *UndoJournal) Stale() []StaleEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]StaleEntry(nil), j.stale...)
}

// Dir is the artifact root ("" for a memory-only journal); used by tests.
func (j *UndoJournal) Dir() string { return j.dir }

// Close is the clean-exit lifecycle boundary: it GCs every crash artifact
// under dir (entries die with the process; the disk blobs were only for
// crash recovery). Idempotent and nil-safe.
func (j *UndoJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if j.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "entry-") {
			_ = os.RemoveAll(filepath.Join(j.dir, e.Name()))
		}
	}
	return nil
}

// undoFileMeta is one file's on-disk record — the design §4.3 entry
// contract: per-file pre-image blob + {path, mode, existed} metadata +
// post-apply content hash. PostHash is empty until Commit refreshes the meta.
type undoFileMeta struct {
	Requested string `json:"requested"`
	Path      string `json:"path"`
	Mode      uint32 `json:"mode"`
	Existed   bool   `json:"existed"`
	PostHash  string `json:"postHash,omitempty"`
}

type undoMeta struct {
	Seq   int64          `json:"seq"`
	Files []undoFileMeta `json:"files"`
}

// writeMetaLocked fsyncs meta.json for e's entry directory; withPost fills
// the per-file post-apply content hashes (Commit time). Callers hold j.mu.
func (j *UndoJournal) writeMetaLocked(edir string, e *undoEntry, withPost bool) error {
	meta := undoMeta{Seq: e.id, Files: make([]undoFileMeta, len(e.files))}
	for i, f := range e.files {
		m := undoFileMeta{Requested: f.Requested, Path: f.Path, Mode: uint32(f.Mode), Existed: f.Existed}
		if withPost {
			sum := sha256.Sum256(f.Post)
			m.PostHash = hex.EncodeToString(sum[:])
		}
		meta.Files[i] = m
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("undo journal: meta: %w", err)
	}
	if err := fsyncFile(filepath.Join(edir, "meta.json"), raw); err != nil {
		return fmt.Errorf("undo journal: meta: %w", err)
	}
	return nil
}

// entryDirLocked finds the on-disk directory of entry id (prefix scan, same
// scheme as before); "" when absent or memory-only. Callers hold j.mu.
func (j *UndoJournal) entryDirLocked(id int64) string {
	if j.dir == "" {
		return ""
	}
	entries, _ := os.ReadDir(j.dir)
	prefix := "entry-" + strconv.FormatInt(id, 10) + "-"
	for _, d := range entries {
		if d.IsDir() && strings.HasPrefix(d.Name(), prefix) {
			return filepath.Join(j.dir, d.Name())
		}
	}
	return ""
}

// clearRedoLocked invalidates the redo stack (a new mutation makes earlier
// redos non-sequential): every cleared entry leaves the byte accounting and
// its disk artifacts behind with it, exactly as eviction would remove it.
// Callers hold j.mu.
func (j *UndoJournal) clearRedoLocked() {
	for _, r := range j.redo {
		j.bytes -= entryBytes(r)
		if j.bytes < 0 {
			j.bytes = 0
		}
		j.removeDiskLocked(r)
	}
	j.redo = nil
}

// Prepare writes one entry's pre-image blobs and meta to disk and fsyncs
// them, BEFORE the engine performs its first atomicWrite (design §4.2 step
// 5). It returns the entry handle. Memory-only journals skip the disk work.
func (j *UndoJournal) Prepare(files []FileRecord) (*undoEntry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, errors.New("undo journal is closed")
	}
	for i := range files {
		if files[i].Existed && len(files[i].Pre) > maxJournalFileBytes {
			return nil, fmt.Errorf("undo: pre-image for %s exceeds %d-byte cap", files[i].Requested, maxJournalFileBytes)
		}
	}
	j.seq++
	e := &undoEntry{id: j.seq, files: append([]FileRecord(nil), files...)}
	if j.dir == "" {
		return e, nil
	}
	edir := filepath.Join(j.dir, "entry-"+strconv.FormatInt(j.seq, 10)+"-"+randSuffix())
	if err := os.MkdirAll(edir, 0o700); err != nil {
		return nil, fmt.Errorf("undo journal: create %s: %w", edir, err)
	}
	prepared := false
	defer func() {
		if !prepared { // a failed Prepare must not leave a false stale entry
			_ = os.RemoveAll(edir)
		}
	}()
	for i, f := range files {
		if !f.Existed {
			continue
		}
		name := fmt.Sprintf("pre-%d.bin", i)
		if err := fsyncFile(filepath.Join(edir, name), f.Pre); err != nil {
			return nil, fmt.Errorf("undo journal: pre-image %s: %w", name, err)
		}
	}
	// Write-ahead meta (design §4.3 entry contract): per-file {path, mode,
	// existed} plus the pre-image blob set; post-apply hashes stay empty until
	// Commit fsyncs the refreshed meta (post-images do not exist yet).
	if err := j.writeMetaLocked(edir, e, false); err != nil {
		return nil, err
	}
	prepared = true
	return e, nil
}

// Commit pushes a prepared entry onto the undo stack once the engine applied
// every op successfully. It clears the redo stack (a new mutation makes
// earlier redos non-sequential) and enforces the bounds with LRU eviction of
// the oldest entries.
func (j *UndoJournal) Commit(e *undoEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || e == nil {
		return
	}
	j.bytes += entryBytes(e)
	j.undo = append(j.undo, e)
	j.clearRedoLocked() // a new mutation invalidates redo (prior-art precedent)
	// Post-apply hashes land on disk now that every op is applied (§4.3).
	// Best effort: the write-ahead guarantee lives in Prepare; the in-memory
	// entry stays authoritative for /undo and /redo.
	if edir := j.entryDirLocked(e.id); edir != "" {
		_ = j.writeMetaLocked(edir, e, true)
	}
	j.evictLocked()
}

// KeepPartial retains a prepared entry whose apply failed AND whose
// compensating rollback failed (owner decision #8): the tree is partially
// applied and the journal entry stays so a later /undo can retry the restore.
func (j *UndoJournal) KeepPartial(e *undoEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || e == nil {
		return
	}
	e.partial = true
	j.bytes += entryBytes(e)
	j.undo = append(j.undo, e)
	j.clearRedoLocked()
	j.evictLocked()
}

// Discard drops a prepared entry whose apply failed and rolled back cleanly
// (nothing half-applied; no entry survives). Disk artifacts are removed.
func (j *UndoJournal) Discard(e *undoEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e == nil {
		return
	}
	j.removeDiskLocked(e)
}

func entryBytes(e *undoEntry) int64 {
	var n int64
	for _, f := range e.files {
		n += int64(len(f.Pre) + len(f.Post))
	}
	return n
}

func (j *UndoJournal) removeDiskLocked(e *undoEntry) {
	if edir := j.entryDirLocked(e.id); edir != "" {
		_ = os.RemoveAll(edir)
	}
}

// evictLocked drops oldest entries while the count or retained bytes exceed
// the bounds (LRU: the bottom of the undo stack is the least recent). Redo
// entries are kept only while bounds allow; a commit clears them anyway.
func (j *UndoJournal) evictLocked() {
	for (len(j.undo)+len(j.redo)) > maxJournalEntries || j.bytes > maxJournalBytes {
		if len(j.undo) == 0 && len(j.redo) == 0 {
			break
		}
		var victim *undoEntry
		if len(j.undo) > 0 {
			victim = j.undo[0]
			j.undo = j.undo[1:]
		} else {
			victim = j.redo[0]
			j.redo = j.redo[1:]
		}
		j.bytes -= entryBytes(victim)
		if j.bytes < 0 {
			j.bytes = 0
		}
		j.removeDiskLocked(victim)
	}
}

// CanUndo reports whether /undo has an entry.
func (j *UndoJournal) CanUndo() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.undo) > 0
}

// CanRedo reports whether /redo has an entry.
func (j *UndoJournal) CanRedo() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.redo) > 0
}

// UndoResult describes one completed undo/redo: the notice line for the
// status bar plus the per-file restore summary.
type UndoResult struct {
	Action string   // "undo" | "redo" | "undo retry"
	Files  []string // requested paths restored (sorted)
}

// Undo pops the newest entry and reverts it: each file's current content
// must hash-equal the entry's post content (refuse-guard); any mismatch
// refuses the whole undo naming the file (an external edit since the
// mutation is a visible refusal, never a silent clobber). Revert restores
// the pre-image via atomicWrite (or deletes a created file). The entry moves
// to the redo stack with its post-images captured (the symmetric operation).
func (j *UndoJournal) Undo() (UndoResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.undo) == 0 {
		return UndoResult{}, ErrNothingToUndo
	}
	e := j.undo[len(j.undo)-1]
	if e.partial {
		return j.undoPartialLocked(e)
	}
	if err := j.checkPostLocked(e); err != nil {
		return UndoResult{}, err
	}
	// Refuse-guard passed: the current bytes ARE the post content, so the
	// post-images are already captured (files are re-read at redo time from
	// the same bytes). Restore the pre-images.
	for i := range e.files {
		if err := j.revertFileLocked(&e.files[i]); err != nil {
			return UndoResult{}, err
		}
	}
	j.undo = j.undo[:len(j.undo)-1]
	j.redo = append(j.redo, e)
	return UndoResult{Action: "undo", Files: fileRecordRequested(e.files)}, nil
}

// undoPartialLocked retries the restore of a partial entry (apply + rollback
// both failed; owner decision #8): every file is reset to its pre-image (a
// created file is removed), idempotently. The entry is dropped on success —
// there is no clean post state to redo. A second failure keeps the entry and
// reports loudly.
func (j *UndoJournal) undoPartialLocked(e *undoEntry) (UndoResult, error) {
	for i := range e.files {
		if err := j.revertFileLocked(&e.files[i]); err != nil {
			return UndoResult{}, fmt.Errorf("undo retry: %w", err)
		}
	}
	j.undo = j.undo[:len(j.undo)-1]
	j.bytes -= entryBytes(e)
	if j.bytes < 0 {
		j.bytes = 0
	}
	j.removeDiskLocked(e)
	return UndoResult{Action: "undo retry", Files: fileRecordRequested(e.files)}, nil
}

// checkPostLocked is the undo refuse-guard: every file's current content
// must match the post (applied) content recorded at commit. Any mismatch
// refuses the whole undo with the file named. The hint names the two ways a
// file can legitimately diverge from the journal: an approved run_command's
// write (command effects are never journaled — see
// docs/run-command-containment.md) or the user's own edit. Both are visible
// refusals, never a silent clobber.
func (j *UndoJournal) checkPostLocked(e *undoEntry) error {
	for i := range e.files {
		f := &e.files[i]
		cur, err := os.ReadFile(f.Path)
		if err != nil {
			return fmt.Errorf("undo refused: %s: %w (external change since the mutation?)", f.Requested, err)
		}
		if sha256.Sum256(cur) != sha256.Sum256(f.Post) {
			return fmt.Errorf("undo refused: %s changed after the agent mutation — undoing would clobber your edit (an approved run_command or your own change?)", f.Requested)
		}
	}
	return nil
}

// revertFileLocked restores one file to its pre-image, or removes a file the
// mutation created. The removal goes through removeNoFollow (the batch
// rollback primitive): an ancestor swapped to a symlink since the journal
// commit fails the undo closed instead of deleting through the redirected
// path (final-component ENOENT stays tolerated — the removal is idempotent).
func (j *UndoJournal) revertFileLocked(f *FileRecord) error {
	if !f.Existed {
		err := removeNoFollow(f.Path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("undo: remove %s: %w", f.Requested, err)
		}
		return nil
	}
	return atomicWrite(f.Path, f.Pre, f.Mode, "undo", f.Requested)
}

// Redo is the symmetric operation: the newest undone entry's current state
// must equal the state undo left (the pre-images); then the post-images are
// restored and the entry moves back onto the undo stack.
func (j *UndoJournal) Redo() (UndoResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.redo) == 0 {
		return UndoResult{}, ErrNothingToRedo
	}
	e := j.redo[len(j.redo)-1]
	if err := j.checkPreLocked(e); err != nil {
		return UndoResult{}, err
	}
	for i := range e.files {
		f := &e.files[i]
		if err := atomicWrite(f.Path, f.Post, f.Mode, "redo", f.Requested); err != nil {
			return UndoResult{}, fmt.Errorf("redo: %w", err)
		}
	}
	j.redo = j.redo[:len(j.redo)-1]
	j.undo = append(j.undo, e)
	return UndoResult{Action: "redo", Files: fileRecordRequested(e.files)}, nil
}

// checkPreLocked is the redo refuse-guard: the file must still be exactly at
// the state undo left (pre-image present, or absent for a created file).
func (j *UndoJournal) checkPreLocked(e *undoEntry) error {
	for i := range e.files {
		f := &e.files[i]
		if !f.Existed {
			if _, err := os.Stat(f.Path); err == nil {
				return fmt.Errorf("redo refused: %s was recreated after the undo", f.Requested)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("redo refused: %s: %w", f.Requested, err)
			}
			continue
		}
		cur, err := os.ReadFile(f.Path)
		if err != nil {
			return fmt.Errorf("redo refused: %s: %w (changed since the undo?)", f.Requested, err)
		}
		if sha256.Sum256(cur) != sha256.Sum256(f.Pre) {
			return fmt.Errorf("redo refused: %s changed after the undo — redo would clobber your edit", f.Requested)
		}
	}
	return nil
}

// Counts exposes the live stack sizes for the UI ("2 changes").
func (j *UndoJournal) Counts() (undo, redo int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.undo), len(j.redo)
}

func fileRecordRequested(files []FileRecord) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Requested)
	}
	sort.Strings(out)
	return out
}

// fsyncFile writes one crash-artifact file with 0600 permissions and syncs
// it before returning (the write-ahead guarantee the apply engine relies on).
func fsyncFile(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func randSuffix() string {
	return strconv.FormatInt(int64(os.Getpid()), 36) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
