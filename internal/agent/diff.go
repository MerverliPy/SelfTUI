package agent

import (
	"strconv"
	"strings"
)

// Line diff for the write_files review overlay (V2e). The overlay renders a
// one-column unified diff per file with collapsed unchanged context:
//
//	@@ -12,3 +12,4 @@
//	  context line
//	- removed line
//	+ added line
//	··· N unchanged lines ···
//
// The diff is display-oriented and bounded: pathological inputs (thousands of
// lines) degrade to a coarse replace-hunk rather than an unbounded LCS, and
// unchanged runs between hunks are collapsed to a deterministic marker row.
// It is presentation text only — the applied mutation never depends on it.

// diffRow is one rendered diff line. kind is one of "ctx", "del", "add", or
// "gap" (the ··· marker).
type diffRow struct {
	kind string
	text string
}

// maxDiffCells bounds the LCS dynamic-programming table: beyond it the middle
// degrades to a replace hunk. 9000x1000-style mixes stay cheap; two huge
// files each show a coarse replace (correct, just not minimal).
const maxDiffCells = 9_000_000

// lineDiff computes a bounded, deterministic line diff between old and new.
// It returns the rows plus the added/removed line counts (exact regardless of
// any coarse degradation). old/new are whole file contents.
func lineDiff(oldText, newText string) (rows []diffRow, added, removed int) {
	if oldText == newText {
		return nil, 0, 0
	}
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)

	// Trim the common prefix and suffix so the LCS only ever sees the
	// differing middle (agent edits are usually one localized hunk).
	start := 0
	for start < len(oldLines) && start < len(newLines) && oldLines[start] == newLines[start] {
		start++
	}
	endOld, endNew := len(oldLines), len(newLines)
	for endOld > start && endNew > start && oldLines[endOld-1] == newLines[endNew-1] {
		endOld--
		endNew--
	}

	// Rows so far: the common prefix as context.
	rows = appendContext(rows, oldLines[:start])

	if start == endOld && start == endNew {
		return rows, 0, 0
	}

	midOld := oldLines[start:endOld]
	midNew := newLines[start:endNew]

	// Degrade to a replace hunk when an exact LCS would be too expensive.
	if len(midOld)*len(midNew) > maxDiffCells || len(midOld)+len(midNew) > 40_000 {
		for _, l := range midOld {
			rows = append(rows, diffRow{kind: "del", text: l})
			removed++
		}
		for _, l := range midNew {
			rows = append(rows, diffRow{kind: "add", text: l})
			added++
		}
		rows = appendContext(rows, oldLines[endOld:])
		return rows, added, removed
	}

	// Classic LCS over the differing middle.
	lcs := lcsOf(midOld, midNew)
	i, j := 0, 0
	for _, lcsLine := range lcs {
		for i < len(midOld) && midOld[i] != lcsLine {
			rows = append(rows, diffRow{kind: "del", text: midOld[i]})
			removed++
			i++
		}
		for j < len(midNew) && midNew[j] != lcsLine {
			rows = append(rows, diffRow{kind: "add", text: midNew[j]})
			added++
			j++
		}
		if i < len(midOld) {
			rows = append(rows, diffRow{kind: "ctx", text: midOld[i]})
			i++
			j++
		}
	}
	for i < len(midOld) {
		rows = append(rows, diffRow{kind: "del", text: midOld[i]})
		removed++
		i++
	}
	for j < len(midNew) {
		rows = append(rows, diffRow{kind: "add", text: midNew[j]})
		added++
		j++
	}
	rows = appendContext(rows, oldLines[endOld:])
	return rows, added, removed
}

// appendContext appends unchanged suffix lines as context rows.
func appendContext(rows []diffRow, lines []string) []diffRow {
	for _, l := range lines {
		rows = append(rows, diffRow{kind: "ctx", text: l})
	}
	return rows
}

// splitDiffLines splits content into diff lines, dropping the empty element
// a trailing newline produces ("a\n" and "a\n\n" differ; "a\n" is two
// lines, not two lines plus an empty one) — a line terminator, not content.
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	out := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		out = out[:len(out)-1]
	}
	return out
}

// lcsOf returns the longest common subsequence of a and b (equality by line
// text). Bounded callers only; O(n*m) time and memory.
func lcsOf(a, b []string) []string {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			out = append(out, a[i])
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return out
}

// diffContext is the number of unchanged context lines kept around each hunk.
const diffContext = 3

// renderDiffRows renders diff rows into display text with the context
// collapse: unchanged runs longer than 2*diffContext between hunks become
// "··· N unchanged lines ···". The final "… (N more lines)" truncation is the
// overlay's fitContent job, not this renderer's.
func renderDiffRows(rows []diffRow) []string {
	if len(rows) == 0 {
		return nil
	}
	// Cluster the changed rows (del/add) into hunks.
	type hunk struct{ from, to int } // row index range of the changed run
	var hunks []hunk
	i := 0
	for i < len(rows) {
		if rows[i].kind != "del" && rows[i].kind != "add" {
			i++
			continue
		}
		from := i
		for i < len(rows) && (rows[i].kind == "del" || rows[i].kind == "add") {
			i++
		}
		hunks = append(hunks, hunk{from: from, to: i})
	}
	if len(hunks) == 0 {
		return nil
	}

	// Expand each hunk by diffContext rows of context on both sides, merging
	// hunks whose windows touch or overlap, then walk the merged windows.
	type win struct{ from, to int }
	var wins []win
	for _, h := range hunks {
		from := maxInt(0, h.from-diffContext)
		to := minInt(len(rows), h.to+diffContext)
		if n := len(wins); n > 0 && from <= wins[n-1].to {
			wins[n-1].to = maxInt(wins[n-1].to, to)
		} else {
			wins = append(wins, win{from: from, to: to})
		}
	}

	var out []string
	cursor := 0
	for _, w := range wins {
		if w.from > cursor {
			collapsed := w.from - cursor
			if collapsed > 2*diffContext {
				out = append(out, "··· "+strconv.Itoa(collapsed)+" unchanged lines ···")
			} else {
				for _, r := range rows[cursor:w.from] {
					out = append(out, " "+r.text)
				}
			}
		}
		for _, r := range rows[w.from:w.to] {
			switch r.kind {
			case "del":
				out = append(out, "-"+r.text)
			case "add":
				out = append(out, "+"+r.text)
			default:
				out = append(out, " "+r.text)
			}
		}
		cursor = w.to
	}
	if cursor < len(rows) {
		collapsed := len(rows) - cursor
		if collapsed > 2*diffContext {
			out = append(out, "··· "+strconv.Itoa(collapsed)+" unchanged lines ···")
		} else {
			for _, r := range rows[cursor:] {
				out = append(out, " "+r.text)
			}
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
