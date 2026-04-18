# Anomaly Detection — Mixed-DRM Directories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect directories that contain both DRM-free and DRM-protected audio files and surface them on stdout and in `anomalies.txt`.

**Architecture:** All changes are confined to `reporter/`. The `Reporter` accumulates per-directory state during `Record()` calls (which already receive every scan result). At `Finish()` time, it collects mixed directories, prints them to stdout, and writes `anomalies.txt` alongside the existing output files.

**Tech Stack:** Go stdlib (`sort`, `bufio`, `os`, `fmt`, `filepath`) — no new dependencies.

---

## File Map

| File | Change |
|---|---|
| `reporter/reporter.go` | Add `dirFlags` struct, `dirState` field, `MixedDRMDirs` stat, update `Record()` and `Finish()` |
| `reporter/reporter_test.go` | Add 6 new test functions covering all anomaly detection scenarios |

---

### Task 1: Write failing tests for mixed-DRM directory detection

**Files:**
- Modify: `reporter/reporter_test.go`

- [ ] **Step 1: Add the six new test functions**

Append to `reporter/reporter_test.go` (after the existing two test functions):

```go
func TestReporterMixedDRMDirectory(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rep.Record(scanner.Result{Path: "/src/Artist/Album/track1.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Artist/Album/track2.m4p", Category: classifier.CategoryDRMProtected})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if !strings.Contains(string(data), "Artist/Album") {
		t.Errorf("anomalies.txt missing expected dir; got: %q", string(data))
	}
}

func TestReporterNoMixedDRMDirectory(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// All-free dir and all-protected dir: neither is flagged.
	rep.Record(scanner.Result{Path: "/src/Free/track1.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Free/track2.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Protected/show.m4p", Category: classifier.CategoryDRMProtected})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 0 {
		t.Errorf("MixedDRMDirs = %d, want 0", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Errorf("anomalies.txt should be empty; got: %q", string(data))
	}
}

func TestReporterSkipFilesIgnoredForMixedDRM(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// One DRM-free track + artwork (skip) in same dir: not mixed.
	rep.Record(scanner.Result{Path: "/src/Album/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Album/cover.jpg", Category: classifier.CategorySkip})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 0 {
		t.Errorf("MixedDRMDirs = %d, want 0 (skip files must not count)", stats.MixedDRMDirs)
	}
}

func TestReporterMixedDRMMultipleDirs(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Dir A: mixed — flagged.
	rep.Record(scanner.Result{Path: "/src/A/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/A/track.m4p", Category: classifier.CategoryDRMProtected})
	// Dir B: all free — not flagged.
	rep.Record(scanner.Result{Path: "/src/B/track.mp3", Category: classifier.CategoryDRMFree})
	// Dir C: mixed — flagged.
	rep.Record(scanner.Result{Path: "/src/C/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/C/track.m4p", Category: classifier.CategoryDRMProtected})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 2 {
		t.Errorf("MixedDRMDirs = %d, want 2", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "A") {
		t.Errorf("anomalies.txt missing dir A; got: %q", content)
	}
	if !strings.Contains(content, "C") {
		t.Errorf("anomalies.txt missing dir C; got: %q", content)
	}
	if strings.Contains(content, "B") {
		t.Errorf("anomalies.txt must not contain dir B; got: %q", content)
	}
}

func TestReporterRootFilesAreMixed(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Files directly under /src — rel parent is ".".
	rep.Record(scanner.Result{Path: "/src/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/track.m4p", Category: classifier.CategoryDRMProtected})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1 (root dir must be flagged)", stats.MixedDRMDirs)
	}
}

func TestReporterDryRunAnomaliesFileNotWritten(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst", DryRun: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rep.Record(scanner.Result{Path: "/src/Album/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Album/track.m4p", Category: classifier.CategoryDRMProtected})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1", stats.MixedDRMDirs)
	}
	if _, err := os.Stat(filepath.Join(dir, "anomalies.txt")); !os.IsNotExist(err) {
		t.Error("anomalies.txt must not exist in dry-run mode")
	}
}
```

- [ ] **Step 2: Run the new tests to confirm they fail**

```bash
go test ./reporter/... -run "TestReporterMixedDRM|TestReporterNoMixed|TestReporterSkip|TestReporterRoot|TestReporterDryRunAnomalies" -v
```

Expected: all six tests FAIL — `stats.MixedDRMDirs` is undefined and `anomalies.txt` does not exist.

---

### Task 2: Implement mixed-DRM directory detection in Reporter

**Files:**
- Modify: `reporter/reporter.go`

- [ ] **Step 1: Add `sort` to the import block**

Current imports:
```go
import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"

	"github.com/ianchesal/itunes-detangler/classifier"
	"github.com/ianchesal/itunes-detangler/scanner"
)
```

Replace with:
```go
import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/schollz/progressbar/v3"

	"github.com/ianchesal/itunes-detangler/classifier"
	"github.com/ianchesal/itunes-detangler/scanner"
)
```

- [ ] **Step 2: Add `dirFlags` struct and `MixedDRMDirs` to `Stats`**

After the closing brace of the `Stats` struct, add the `dirFlags` struct. Also add `MixedDRMDirs` to `Stats`.

Current `Stats`:
```go
type Stats struct {
	Total        int
	DRMFree      int
	DRMProtected int
	Skipped      int
	Errors       int
}
```

Replace with:
```go
type Stats struct {
	Total        int
	DRMFree      int
	DRMProtected int
	Skipped      int
	Errors       int
	MixedDRMDirs int
}

type dirFlags struct {
	hasFree bool
	hasProt bool
}
```

- [ ] **Step 3: Add `dirState` field to `Reporter`**

Current `Reporter` struct:
```go
type Reporter struct {
	cfg        Config
	bar        *progressbar.ProgressBar
	freeWriter io.Writer
	protWriter io.Writer
	freeFile   *os.File
	protFile   *os.File
	stats      Stats
	writeErr   error // first write error encountered; returned by Finish
}
```

Replace with:
```go
type Reporter struct {
	cfg        Config
	bar        *progressbar.ProgressBar
	freeWriter io.Writer
	protWriter io.Writer
	freeFile   *os.File
	protFile   *os.File
	stats      Stats
	dirState   map[string]*dirFlags
	writeErr   error // first write error encountered; returned by Finish
}
```

- [ ] **Step 4: Initialize `dirState` in `New()`**

Current initialization line:
```go
r := &Reporter{cfg: cfg, bar: bar}
```

Replace with:
```go
r := &Reporter{cfg: cfg, bar: bar, dirState: make(map[string]*dirFlags)}
```

- [ ] **Step 5: Update `Record()` to track directory state**

Current `Record()`:
```go
func (r *Reporter) Record(result scanner.Result) {
	r.bar.Add(1)
	r.stats.Total++

	rel, err := filepath.Rel(r.cfg.SrcPath, result.Path)
	if err != nil {
		// Can't make path relative to SrcPath — writing an absolute path would
		// silently produce wrong rsync behaviour, so skip it.
		r.stats.Skipped++
		return
	}

	switch result.Category {
	case classifier.CategoryDRMFree:
		r.stats.DRMFree++
		if _, err := fmt.Fprintln(r.freeWriter, rel); err != nil && r.writeErr == nil {
			r.writeErr = err
		}
	case classifier.CategoryDRMProtected:
		r.stats.DRMProtected++
		if _, err := fmt.Fprintln(r.protWriter, rel); err != nil && r.writeErr == nil {
			r.writeErr = err
		}
	default:
		r.stats.Skipped++
	}
}
```

Replace with:
```go
func (r *Reporter) Record(result scanner.Result) {
	r.bar.Add(1)
	r.stats.Total++

	rel, err := filepath.Rel(r.cfg.SrcPath, result.Path)
	if err != nil {
		// Can't make path relative to SrcPath — writing an absolute path would
		// silently produce wrong rsync behaviour, so skip it.
		r.stats.Skipped++
		return
	}

	dir := filepath.Dir(rel)
	switch result.Category {
	case classifier.CategoryDRMFree:
		r.stats.DRMFree++
		if _, err := fmt.Fprintln(r.freeWriter, rel); err != nil && r.writeErr == nil {
			r.writeErr = err
		}
		r.markDir(dir, true, false)
	case classifier.CategoryDRMProtected:
		r.stats.DRMProtected++
		if _, err := fmt.Fprintln(r.protWriter, rel); err != nil && r.writeErr == nil {
			r.writeErr = err
		}
		r.markDir(dir, false, true)
	default:
		r.stats.Skipped++
	}
}

func (r *Reporter) markDir(dir string, free, prot bool) {
	f := r.dirState[dir]
	if f == nil {
		f = &dirFlags{}
		r.dirState[dir] = f
	}
	if free {
		f.hasFree = true
	}
	if prot {
		f.hasProt = true
	}
}
```

- [ ] **Step 6: Update `Finish()` to collect, print, and write anomalies**

Current `Finish()` (the section after `r.bar.Finish()` and before `return`):
```go
	r.bar.Finish()

	fmt.Printf("\nScan complete: %d files | %d owned | %d DRM-protected | %d skipped | %d errors\n",
		r.stats.Total, r.stats.DRMFree, r.stats.DRMProtected, r.stats.Skipped, r.stats.Errors)

	if !r.cfg.DryRun {
		freePath, _ := filepath.Abs(filepath.Join(r.cfg.OutDir, "drm-free.txt"))
		fmt.Printf("\nrsync -av --files-from=%s %s %s\n",
			shellQuote(freePath), shellQuote(r.cfg.SrcPath), shellQuote(r.cfg.DestPath))
	}

	return r.stats, r.writeErr
```

Replace with:
```go
	r.bar.Finish()

	// Collect mixed-DRM directories.
	var mixed []string
	for d, f := range r.dirState {
		if f.hasFree && f.hasProt {
			mixed = append(mixed, d)
		}
	}
	sort.Strings(mixed)
	r.stats.MixedDRMDirs = len(mixed)

	fmt.Printf("\nScan complete: %d files | %d owned | %d DRM-protected | %d skipped | %d errors\n",
		r.stats.Total, r.stats.DRMFree, r.stats.DRMProtected, r.stats.Skipped, r.stats.Errors)

	if len(mixed) > 0 {
		noun := "directories"
		if len(mixed) == 1 {
			noun = "directory"
		}
		fmt.Printf("\nAnomalies: %d mixed-DRM %s\n", len(mixed), noun)
		for _, d := range mixed {
			fmt.Printf("  %s\n", d)
		}
	}

	if !r.cfg.DryRun {
		// Write anomalies.txt — always, even if empty, for predictability.
		anomPath := filepath.Join(r.cfg.OutDir, "anomalies.txt")
		af, err := os.Create(anomPath)
		if err != nil && r.writeErr == nil {
			r.writeErr = err
		} else if err == nil {
			aw := bufio.NewWriter(af)
			for _, d := range mixed {
				if _, werr := fmt.Fprintln(aw, d); werr != nil && r.writeErr == nil {
					r.writeErr = werr
				}
			}
			if ferr := aw.Flush(); ferr != nil && r.writeErr == nil {
				r.writeErr = ferr
			}
			af.Close()
		}

		freePath, _ := filepath.Abs(filepath.Join(r.cfg.OutDir, "drm-free.txt"))
		fmt.Printf("\nrsync -av --files-from=%s %s %s\n",
			shellQuote(freePath), shellQuote(r.cfg.SrcPath), shellQuote(r.cfg.DestPath))
	}

	return r.stats, r.writeErr
```

---

### Task 3: Run all tests and commit

**Files:** none

- [ ] **Step 1: Run the full reporter test suite**

```bash
go test ./reporter/... -v
```

Expected: all tests PASS, including the six new ones and the two pre-existing ones.

- [ ] **Step 2: Run the full project test suite**

```bash
go test ./...
```

Expected: all packages PASS with no failures.

- [ ] **Step 3: Verify the binary builds cleanly**

```bash
go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 4: Commit**

```bash
git add reporter/reporter.go reporter/reporter_test.go
git commit -m "feat: detect and report mixed-DRM directories as anomalies"
```
