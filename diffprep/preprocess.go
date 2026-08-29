// Package diffprep filters, summarizes, and prepares git diffs before sending
// them to the LLM API. It handles:
//
//   - Filtering out noise files (lock files, auto-generated, binary)
//   - Computing per-file stats for context awareness
//   - Truncating extremely large diffs while preserving structure
//   - Preparing chunked output for hierarchical summarization
package diffprep

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// NoisePatterns are file patterns that should be excluded from diff analysis.
//
// Why filter at all: in real projects, lock files, build output, and
// generated code routinely dominate a diff's line count. Sending them
// to the model burns context budget on lines that carry no intent, and
// it actively hurts output quality — a commit message that mentions
// "updated package-lock.json" is worse than one that doesn't.
//
// Patterns are intentionally conservative (only obvious noise).
// Excluding something by mistake is recoverable (the file is still in
// the diff the user sees); including real code by mistake is not.
var NoisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\.lock$`),               // package-lock.json, Gemfile.lock
	regexp.MustCompile(`(?i)^node_modules/`),        // node_modules
	regexp.MustCompile(`(?i)\.min\.(js|css)$`),      // minified files
	regexp.MustCompile(`(?i)^vendor/`),              // Go vendor dir
	regexp.MustCompile(`(?i)\.pb\.go$`),             // protobuf generated
	regexp.MustCompile(`(?i)\.generated\.`),         // auto-generated files
	regexp.MustCompile(`(?i)\.svg$`),                // SVGs (diff is noise)
	regexp.MustCompile(`(?i)\.png$|\.jpg$|\.jpeg$`), // images (binary)
	regexp.MustCompile(`(?i)\.git/`),                // git internals
	regexp.MustCompile(`(?i)go\.sum$`),              // Go dependency checksums
	regexp.MustCompile(`(?i)yarn\.lock$`),           // yarn lockfile
	regexp.MustCompile(`(?i)poetry\.lock$`),         // poetry lockfile
	regexp.MustCompile(`(?i)Pipfile\.lock$`),        // pip lockfile
	regexp.MustCompile(`(?i)\.bundle/`),             // bundler cache
	regexp.MustCompile(`(?i)dist/`),                 // build output
	regexp.MustCompile(`(?i)build/`),                // build output
	regexp.MustCompile(`(?i)\.next/`),               // next.js build cache
	regexp.MustCompile(`(?i)\.d\.ts$`),              // TypeScript declaration files
	regexp.MustCompile(`(?i)\.map$`),                // source maps
}

// FileStats tracks metadata about a single file in the diff.
//
// The boolean flags are derived from the git diff header lines git
// emits per file ("new file mode", "deleted file mode", "similarity
// index"), and the counters from scanning the + / - lines. RawDiff is
// the full per-file chunk kept around so chunking (ChunkDiff) can work
// on the original text without re-parsing.
type FileStats struct {
	Filename  string
	IsNewFile bool
	IsDeleted bool
	IsRenamed bool
	Additions int
	Deletions int
	IsBinary  bool
	IsNoise   bool
	RawDiff   string
}

// PreparedDiff is the result of preprocessing a raw git diff.
type PreparedDiff struct {
	Files              []FileStats
	TotalFiles         int
	TotalAdded         int
	TotalDeleted       int
	TotalNoiseFiltered int
	TotalBinarySkipped int
	Summary            string // human-readable summary of the diff
	Content            string // the actual diff content to send to the model
}

// Process takes a raw git diff string and returns a prepared, filtered diff.
//
// Pipeline: split into per-file chunks → drop binary files → drop noise
// files → cap each surviving file at a line budget → assemble a
// summary header plus the cleaned content. The returned Content is what
// actually gets sent to the model; Summary is a cheap human/model
// orientation block ("3 files, +120 -40") that costs almost no tokens
// but orients the model before the hunks start.
func Process(rawDiff string) *PreparedDiff {
	files := parseDiff(rawDiff)

	result := &PreparedDiff{}
	var contentBuf bytes.Buffer

	for i := range files {
		f := &files[i]

		// Binary files first: they have no textual diff at all, so
		// there is nothing to filter or count.
		if f.IsBinary = isBinary(f.RawDiff); f.IsBinary {
			result.TotalBinarySkipped++
			continue
		}

		// Noise files are dropped by filename, before any of their
		// content is counted, so the totals below reflect only real
		// code.
		f.IsNoise = isNoiseFile(f.Filename)
		if f.IsNoise {
			result.TotalNoiseFiltered++
			continue
		}

		result.Files = append(result.Files, *f)
		result.TotalFiles++
		result.TotalAdded += f.Additions
		result.TotalDeleted += f.Deletions

		// Truncate individual file diffs if they're excessively large.
		// 2000 lines per file is far beyond what any single change
		// legitimately needs in a prompt; anything larger is usually
		// a generated-file regression and would crowd out the rest of
		// the diff.
		truncated := truncateFileDiff(f.RawDiff, f.Filename, 2000)
		contentBuf.WriteString(truncated)
		contentBuf.WriteString("\n\n")
	}

	// Build a summary header for model context.
	result.Summary = buildSummary(result)
	result.Content = result.Summary + "\n" + contentBuf.String()

	return result
}

// parseDiff splits a unified git diff into per-file chunks.
//
// The "diff --git a/... b/..." header line is the only reliable file
// boundary in unified format, so we locate every occurrence and slice
// the raw diff between consecutive matches.
func parseDiff(rawDiff string) []FileStats {
	var files []FileStats

	// Split on "diff --git" boundaries.
	//
	// FindAllStringSubmatchIndex (rather than FindAllStringSubmatch) is
	// used because we need byte offsets, not the matched strings: the
	// offset of each match tells us where one file's chunk ends and the
	// next begins, while the captured-group offsets give us the a/ and
	// b/ paths. One call gets all three.
	filePattern := regexp.MustCompile(`(?m)^diff --git a/(.*) b/(.*)$`)
	matches := filePattern.FindAllStringSubmatchIndex(rawDiff, -1)

	if len(matches) == 0 {
		return files
	}

	for i, m := range matches {
		// m[0:1] = overall match, m[2:3] = first capture (a/...), m[4:5] = second capture (b/...)
		start := m[0]
		end := len(rawDiff)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}

		chunk := rawDiff[start:end]

		var aPath, bPath string
		if m[2] >= 0 && m[3] > m[2] {
			aPath = rawDiff[m[2]:m[3]]
		}
		if m[4] >= 0 && m[5] > m[4] {
			bPath = rawDiff[m[4]:m[5]]
		}

		fs := parseFileChunk(chunk, aPath, bPath)
		files = append(files, fs)
	}

	return files
}

// parseFileChunk extracts stats from a single file's diff chunk.
func parseFileChunk(chunk, aPath, bPath string) FileStats {
	fs := FileStats{
		Filename: normalizeFilename(aPath, bPath),
		RawDiff:  chunk,
	}

	// Detect file status from the git header lines git emits at the
	// top of each chunk. These are plain-text sentinels (not structured
	// fields), so matching on their presence is the simplest robust
	// approach. "index 0000000.." (an all-zero source object) is the
	// reliable marker of a brand-new file.
	if strings.Contains(chunk, "new file mode") || strings.Contains(chunk, "index 0000000..") {
		fs.IsNewFile = true
	}
	if strings.Contains(chunk, "deleted file mode") {
		fs.IsDeleted = true
	}
	if strings.Contains(chunk, "similarity index") {
		fs.IsRenamed = true
	}

	// Count additions and deletions.
	scanDiff(chunk, &fs)

	return fs
}

// scanDiff counts + and - lines in a diff chunk.
//
// It walks the chunk line by line and counts lines that start with +
// (addition) or - (deletion). The +++ / --- guards are essential: the
// per-file header lines ("--- a/path", "+++ b/path") also start with
// those characters but are not content changes, so without the guard
// every file would be off by two.
func scanDiff(chunk string, fs *FileStats) {
	scanner := bufio.NewScanner(strings.NewReader(chunk))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && line[0] == '+' && !strings.HasPrefix(line, "+++") {
			fs.Additions++
		} else if len(line) > 0 && line[0] == '-' && !strings.HasPrefix(line, "---") {
			fs.Deletions++
		}
	}
}

// isBinary checks if the diff chunk represents a binary file.
func isBinary(rawDiff string) bool {
	return strings.Contains(rawDiff, "Binary files ") ||
		strings.Contains(rawDiff, "Binary file")
}

// isNoiseFile checks if the filename matches any noise pattern.
func isNoiseFile(filename string) bool {
	for _, pattern := range NoisePatterns {
		if pattern.MatchString(filename) {
			return true
		}
	}
	return false
}

// normalizeFilename returns the most useful filename from git diff paths.
//
// Git represents "no such file" as the literal path /dev/null, so
// create/delete/rename all show up as a pair where one side is
// dev/null. The cases are:
//
//   - both real and different → rename, report the new (b) name
//   - a is dev/null           → new file, report b
//   - b is dev/null           → deleted file, report a
//   - otherwise               → plain modification, report a
func normalizeFilename(aPath, bPath string) string {
	if aPath != bPath && aPath != "dev/null" && bPath != "dev/null" {
		return bPath // renamed: use the new name
	}
	if aPath == "dev/null" {
		return bPath // new file
	}
	if bPath == "dev/null" {
		return aPath // deleted file
	}
	return aPath
}

// truncateFileDiff truncates a single file's diff to maxLines lines,
// preserving the header and showing a summary of skipped content.
//
// The strategy is to keep the first 50 and last 50 lines and elide the
// middle with a marker. Rationale: the top of a diff carries the file
// header, the new/old mode lines, and the first hunks (where the change
// usually starts), while the tail shows the final state. For very large
// generated-ish files neither end is as important as the *fact* that it
// changed, which the elision marker conveys ("N lines truncated").
// This keeps a single pathological file from consuming the entire
// context budget while still telling the model what happened.
func truncateFileDiff(rawDiff string, filename string, maxLines int) string {
	lines := strings.Split(rawDiff, "\n")
	if len(lines) <= maxLines {
		return rawDiff
	}

	// Keep first 50 lines (file headers + context), truncate middle, keep last 50.
	headerLines := 50
	footerLines := 50
	skipped := len(lines) - headerLines - footerLines

	var buf bytes.Buffer
	buf.WriteString(strings.Join(lines[:headerLines], "\n"))
	// fmt.Fprintf writes the formatted marker straight into the buffer
	// (no intermediate string from Sprintf + WriteString).
	fmt.Fprintf(&buf, "\n\\ No newline at end of file\n\\ ... %d lines of diff truncated ...\n\\ %s\n", skipped, filename)
	buf.WriteString(strings.Join(lines[len(lines)-footerLines:], "\n"))

	return buf.String()
}

// buildSummary creates a concise summary of the diff for model context.
//
// It is intentionally compact: a few aggregate counts plus one line per
// file with its status and +/-. This gives the model a fast mental map
// ("how many files, which changed most") before it reads the full
// hunks, which measurably improves the focus of the generated output
// without adding much token cost.
func buildSummary(pd *PreparedDiff) string {
	if len(pd.Files) == 0 {
		return ""
	}

	// Sort files by change density (most changes first) so the model
	// sees the highest-signal files at the top of the summary, matching
	// where a human reviewer's eye lands first.
	sorted := make([]FileStats, len(pd.Files))
	copy(sorted, pd.Files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Additions+sorted[i].Deletions > sorted[j].Additions+sorted[j].Deletions
	})

	var sb strings.Builder
	// fmt.Fprintf writes formatted lines straight into the builder.
	fmt.Fprintf(&sb, "=== Diff Summary ===\n")
	fmt.Fprintf(&sb, "Files changed: %d\n", pd.TotalFiles)
	fmt.Fprintf(&sb, "Total insertions: %d\n", pd.TotalAdded)
	fmt.Fprintf(&sb, "Total deletions: %d\n", pd.TotalDeleted)
	if pd.TotalNoiseFiltered > 0 {
		fmt.Fprintf(&sb, "(Filtered out %d noise files, %d binary files)\n", pd.TotalNoiseFiltered, pd.TotalBinarySkipped)
	}
	sb.WriteString("\nFiles:\n")

	for _, f := range sorted {
		status := "modified"
		if f.IsNewFile {
			status = "new"
		} else if f.IsDeleted {
			status = "deleted"
		} else if f.IsRenamed {
			status = "renamed"
		}
		fmt.Fprintf(&sb, "  [%s] %s (+%d -%d)\n", status, f.Filename, f.Additions, f.Deletions)
	}

	return sb.String()
}

// ShouldChunk returns true if the diff is large enough to benefit from
// chunked/hierarchical summarization instead of a single API call.
//
// 500 lines is the rough point where a single call risks either
// exceeding a modest model's comfortable context or losing focus across
// too many files. Below it, one call is simpler and just as good; above
// it, the two-stage pipeline (see commands.Commit) pays for itself.
func ShouldChunk(rawDiff string) bool {
	lines := strings.Count(rawDiff, "\n")
	return lines > 500 // More than ~500 lines = worth chunking
}

// ChunkDiff splits a raw diff into manageable chunks. Each chunk contains
// one or more COMPLETE file diffs — a file is never split across two
// chunks, because a partial diff (missing its header or trailing hunk)
// is harder for the model to interpret than a slightly larger whole one.
//
// It greedily fills each chunk up to maxChunkLines; a single file that
// is itself larger than the budget gets its own chunk rather than being
// truncated here (per-file truncation is Process's job, applied later).
func ChunkDiff(rawDiff string, maxChunkLines int) [][]FileStats {
	files := parseDiff(rawDiff)
	if maxChunkLines <= 0 {
		maxChunkLines = 300
	}

	var chunks [][]FileStats
	var currentChunk []FileStats
	currentLines := 0

	for _, f := range files {
		fileLines := strings.Count(f.RawDiff, "\n")

		// If adding this file would exceed the chunk limit and we already have files,
		// flush the current chunk
		if currentLines+fileLines > maxChunkLines && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentLines = 0
		}

		currentChunk = append(currentChunk, f)
		currentLines += fileLines
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// ChunkToDiff converts a group of FileStats back into a diff string.
func ChunkToDiff(files []FileStats) string {
	var buf bytes.Buffer
	for _, f := range files {
		buf.WriteString(f.RawDiff)
		buf.WriteString("\n\n")
	}
	return buf.String()
}
