# itunes-detangler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a macOS CLI tool in Go that scans an iTunes library directory, classifies tracks as DRM-free, DRM-protected, or skip, and produces rsync-ready output files with a progress bar and incremental SQLite cache.

**Architecture:** A concurrent scanner walks the directory tree with a configurable worker pool; each worker checks a SQLite cache before invoking the file classifier; an aggregator goroutine collects results and writes two output files. MP4 DRM detection reads only the box header tree — no audio data is loaded.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite` (pure-Go, no CGo), `schollz/progressbar/v3`

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | CLI flags, component wiring, graceful shutdown |
| `classifier/classifier.go` | `Category` type, `Classify(path)` public function |
| `classifier/mp4.go` | MP4 box parser, DRM detection for `.m4a` files |
| `classifier/classifier_test.go` | Extension and M4A classification tests |
| `cache/cache.go` | SQLite open / lookup / upsert / reset |
| `cache/cache_test.go` | Cache CRUD tests |
| `scanner/scanner.go` | Directory walker + worker pool |
| `scanner/scanner_test.go` | Scanner integration tests |
| `reporter/reporter.go` | Output file writer, progress bar, rsync command |
| `reporter/reporter_test.go` | Reporter output and stats tests |

---

## Task 1: Project scaffold

**Files:**
- Create: `go.mod`, `go.sum`
- Create: `.gitignore`
- Create: `classifier/classifier.go`, `cache/cache.go`, `scanner/scanner.go`, `reporter/reporter.go`, `main.go`

- [ ] **Step 1: Initialise the Go module**

```bash
cd /home/ichesal/src/ianchesal/itunes-detangler
git init
go mod init github.com/ianchesal/itunes-detangler
```

Expected: `go.mod` created with `module github.com/ianchesal/itunes-detangler` and the Go version line.

- [ ] **Step 2: Add dependencies**

```bash
go get modernc.org/sqlite
go get github.com/schollz/progressbar/v3
```

Expected: `go.mod` and `go.sum` updated with both packages and their transitive dependencies.

- [ ] **Step 3: Create `.gitignore`**

```
itunes-detangler
*.txt
*.db
```

- [ ] **Step 4: Create empty package stubs**

`classifier/classifier.go`:
```go
package classifier
```

`cache/cache.go`:
```go
package cache
```

`scanner/scanner.go`:
```go
package scanner
```

`reporter/reporter.go`:
```go
package reporter
```

`main.go`:
```go
package main

func main() {}
```

- [ ] **Step 5: Verify build**

```bash
go build ./...
```

Expected: no output, no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum .gitignore classifier/classifier.go cache/cache.go scanner/scanner.go reporter/reporter.go main.go docs/
git commit -m "chore: project scaffold"
```

---

## Task 2: Classifier — category types and extension detection

**Files:**
- Modify: `classifier/classifier.go`
- Create: `classifier/classifier_test.go`

- [ ] **Step 1: Write failing tests**

Create `classifier/classifier_test.go`:
```go
package classifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyDRMFreeExtensions(t *testing.T) {
	for _, ext := range []string{".mp3", ".flac", ".aiff", ".aif", ".wav"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("Classify: unexpected error: %v", err)
			}
			if got != CategoryDRMFree {
				t.Errorf("Classify(%q) = %v, want %v", filepath.Ext(f.Name()), got, CategoryDRMFree)
			}
		})
	}
}

func TestClassifyM4PIsProtected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.m4p")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMProtected {
		t.Errorf("Classify(.m4p) = %v, want CategoryDRMProtected", got)
	}
}

func TestClassifyUnknownExtensionIsSkip(t *testing.T) {
	for _, ext := range []string{".jpg", ".pdf", ".xml", ".nfo"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != CategorySkip {
				t.Errorf("Classify(%q) = %v, want CategorySkip", ext, got)
			}
		})
	}
}

func TestClassifyIsCaseInsensitive(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.MP3")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("Classify(.MP3) = %v, want CategoryDRMFree", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./classifier/... -v -run TestClassify
```

Expected: compile error — `Classify`, `CategoryDRMFree`, etc. not defined.

- [ ] **Step 3: Implement category type and Classify**

Replace `classifier/classifier.go`:
```go
package classifier

import (
	"path/filepath"
	"strings"
)

// Category is the classification of a music file.
type Category int

const (
	CategorySkip         Category = iota // non-music, artwork, or unrecognised
	CategoryDRMFree                      // owned, freely copyable
	CategoryDRMProtected                 // owned but Fairplay-protected
)

func (c Category) String() string {
	switch c {
	case CategoryDRMFree:
		return "drm-free"
	case CategoryDRMProtected:
		return "drm-protected"
	default:
		return "skip"
	}
}

// Classify returns the category for the file at path.
// .m4a files are inspected via MP4 box headers; all other formats are
// determined by extension alone.
func Classify(path string) (Category, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac", ".aiff", ".aif", ".wav":
		return CategoryDRMFree, nil
	case ".m4p":
		return CategoryDRMProtected, nil
	case ".m4a":
		return classifyM4A(path)
	default:
		return CategorySkip, nil
	}
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./classifier/... -v -run TestClassify
```

Expected: all four test functions PASS. (M4A tests come in Task 3.)

- [ ] **Step 5: Commit**

```bash
git add classifier/
git commit -m "feat: classifier — Category type and extension detection"
```

---

## Task 3: Classifier — MP4 DRM detection

**Files:**
- Create: `classifier/mp4.go`
- Modify: `classifier/classifier_test.go` (add M4A test helpers and tests)

- [ ] **Step 1: Write failing M4A tests**

Add the following to the **bottom** of `classifier/classifier_test.go`. Also add `"encoding/binary"` to the import block.

```go
// --- MP4 test fixture helpers ---

func buildBox(boxType string, content []byte) []byte {
	result := make([]byte, 8+len(content))
	binary.BigEndian.PutUint32(result[:4], uint32(8+len(content)))
	copy(result[4:8], boxType)
	copy(result[8:], content)
	return result
}

func buildAudioEntry(codec string, hasSinf bool) []byte {
	// AudioSampleEntry fixed fields (28 bytes):
	// reserved(6) + dataRefIdx(2) + reserved(8) + channels(2) + sampleSize(2) +
	// compressionId(2) + packetSize(2) + sampleRate(4)
	fields := make([]byte, 28)
	binary.BigEndian.PutUint16(fields[6:8], 1)          // dataRefIdx = 1
	binary.BigEndian.PutUint16(fields[16:18], 2)        // channels = 2
	binary.BigEndian.PutUint16(fields[18:20], 16)       // sampleSize = 16
	binary.BigEndian.PutUint32(fields[24:28], 44100<<16) // sampleRate

	content := append([]byte(nil), fields...)
	if hasSinf {
		content = append(content, buildBox("sinf", []byte{0})...)
	}
	return buildBox(codec, content)
}

func buildM4AData(codec string, hasSinf bool) []byte {
	entry := buildAudioEntry(codec, hasSinf)

	// stsd preamble: version(1)+flags(3)+entryCount(4) = 8 bytes
	stsdContent := make([]byte, 8)
	binary.BigEndian.PutUint32(stsdContent[4:8], 1) // entryCount = 1
	stsdContent = append(stsdContent, entry...)

	stsd := buildBox("stsd", stsdContent)
	stbl := buildBox("stbl", stsd)
	minf := buildBox("minf", stbl)
	mdia := buildBox("mdia", minf)
	trak := buildBox("trak", mdia)
	return buildBox("moov", trak)
}

func writeTempM4A(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.m4a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// --- M4A classification tests ---

func TestClassifyM4ADRMFreeAAC(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("mp4a", false))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("DRM-free AAC .m4a: got %v, want CategoryDRMFree", got)
	}
}

func TestClassifyM4AProtectedAAC(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("mp4a", true))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMProtected {
		t.Errorf("protected AAC .m4a: got %v, want CategoryDRMProtected", got)
	}
}

func TestClassifyM4AAppleLossless(t *testing.T) {
	path := writeTempM4A(t, buildM4AData("alac", false))
	got, err := Classify(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("Apple Lossless .m4a: got %v, want CategoryDRMFree", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./classifier/... -v -run TestClassifyM4A
```

Expected: compile error — `classifyM4A` not defined.

- [ ] **Step 3: Implement the MP4 box parser**

Create `classifier/mp4.go`:
```go
package classifier

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var errBoxNotFound = errors.New("mp4: box not found")

type boxHeader struct {
	size    uint32 // total box size including the 8-byte header
	boxType string
}

func readBoxHeader(r io.Reader) (boxHeader, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return boxHeader{}, err
	}
	return boxHeader{
		size:    binary.BigEndian.Uint32(buf[:4]),
		boxType: string(buf[4:8]),
	}, nil
}

// findBox scans for a box of the given type within limit bytes of the current
// reader position. On success r is positioned at the start of that box's content.
// Returns the content size (box size minus the 8-byte header).
func findBox(r io.ReadSeeker, target string, limit int64) (int64, error) {
	var consumed int64
	for consumed < limit {
		h, err := readBoxHeader(r)
		if err != nil {
			return 0, err
		}
		consumed += int64(h.size)
		if h.boxType == target {
			return int64(h.size) - 8, nil
		}
		if _, err := r.Seek(int64(h.size)-8, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	return 0, errBoxNotFound
}

func classifyM4A(path string) (Category, error) {
	f, err := os.Open(path)
	if err != nil {
		return CategorySkip, err
	}
	defer f.Close()

	totalSize, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return CategorySkip, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return CategorySkip, err
	}
	return classifyM4AReader(f, totalSize)
}

// classifyM4AReader is the testable core of classifyM4A.
func classifyM4AReader(r io.ReadSeeker, totalSize int64) (Category, error) {
	moovSize, err := findBox(r, "moov", totalSize)
	if err != nil {
		return CategorySkip, nil // not a valid MP4 — skip silently
	}
	trakSize, err := findBox(r, "trak", moovSize)
	if err != nil {
		return CategorySkip, nil
	}
	mdiaSize, err := findBox(r, "mdia", trakSize)
	if err != nil {
		return CategorySkip, nil
	}
	minfSize, err := findBox(r, "minf", mdiaSize)
	if err != nil {
		return CategorySkip, nil
	}
	stblSize, err := findBox(r, "stbl", minfSize)
	if err != nil {
		return CategorySkip, nil
	}
	if _, err := findBox(r, "stsd", stblSize); err != nil {
		return CategorySkip, nil
	}

	// Skip stsd preamble: version(1) + flags(3) + entryCount(4) = 8 bytes
	if _, err := r.Seek(8, io.SeekCurrent); err != nil {
		return CategorySkip, nil
	}

	entryHdr, err := readBoxHeader(r)
	if err != nil {
		return CategorySkip, nil
	}
	entryContentSize := int64(entryHdr.size) - 8

	switch entryHdr.boxType {
	case "alac":
		return CategoryDRMFree, nil
	case "mp4a":
		// Skip AudioSampleEntry fixed fields (28 bytes) before any child boxes.
		// Fields: reserved(6)+dataRefIdx(2)+reserved(8)+channels(2)+sampleSize(2)+
		//         compressionId(2)+packetSize(2)+sampleRate(4) = 28 bytes
		if _, err := r.Seek(28, io.SeekCurrent); err != nil {
			return CategorySkip, nil
		}
		_, sinfErr := findBox(r, "sinf", entryContentSize-28)
		if errors.Is(sinfErr, errBoxNotFound) {
			return CategoryDRMFree, nil
		}
		if sinfErr != nil {
			return CategorySkip, nil
		}
		return CategoryDRMProtected, nil
	default:
		return CategorySkip, nil
	}
}
```

- [ ] **Step 4: Run all classifier tests**

```bash
go test ./classifier/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add classifier/
git commit -m "feat: MP4 DRM detection via box header inspection"
```

---

## Task 4: Cache — SQLite store

**Files:**
- Modify: `cache/cache.go`
- Create: `cache/cache_test.go`

- [ ] **Step 1: Write failing tests**

Create `cache/cache_test.go`:
```go
package cache

import (
	"testing"

	"github.com/ianchesal/itunes-detangler/classifier"
)

func openMemory(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestLookupMiss(t *testing.T) {
	c := openMemory(t)
	_, ok, err := c.Lookup("/some/path.mp3", 1000, 512)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on empty cache, got hit")
	}
}

func TestUpsertAndLookupHit(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/music/track.mp3", Mtime: 1700000000, Size: 4096000, Category: classifier.CategoryDRMFree}
	if err := c.Upsert(e); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok, err := c.Lookup(e.Path, e.Mtime, e.Size)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got != classifier.CategoryDRMFree {
		t.Errorf("got %v, want CategoryDRMFree", got)
	}
}

func TestLookupMissOnMtimeChange(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.mp3", Mtime: 100, Size: 500, Category: classifier.CategoryDRMFree}
	c.Upsert(e)
	_, ok, err := c.Lookup(e.Path, 999, e.Size) // different mtime
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on mtime change, got hit")
	}
}

func TestLookupMissOnSizeChange(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.mp3", Mtime: 100, Size: 500, Category: classifier.CategoryDRMFree}
	c.Upsert(e)
	_, ok, err := c.Lookup(e.Path, e.Mtime, 9999) // different size
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on size change, got hit")
	}
}

func TestReset(t *testing.T) {
	c := openMemory(t)
	c.Upsert(Entry{Path: "/a.mp3", Mtime: 1, Size: 1, Category: classifier.CategoryDRMFree})
	c.Upsert(Entry{Path: "/b.mp3", Mtime: 1, Size: 1, Category: classifier.CategoryDRMFree})
	if err := c.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	_, ok, _ := c.Lookup("/a.mp3", 1, 1)
	if ok {
		t.Error("expected empty cache after Reset, got hit")
	}
}

func TestUpsertReplaces(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.m4a", Mtime: 100, Size: 500, Category: classifier.CategorySkip}
	c.Upsert(e)
	e.Category = classifier.CategoryDRMFree
	c.Upsert(e)
	got, ok, err := c.Lookup(e.Path, e.Mtime, e.Size)
	if err != nil || !ok {
		t.Fatalf("Lookup after upsert: err=%v ok=%v", err, ok)
	}
	if got != classifier.CategoryDRMFree {
		t.Errorf("got %v after replace upsert, want CategoryDRMFree", got)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./cache/... -v
```

Expected: compile error — `Cache`, `Open`, `Entry` not defined.

- [ ] **Step 3: Implement the cache**

Replace `cache/cache.go`:
```go
package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/ianchesal/itunes-detangler/classifier"
)

const schema = `
CREATE TABLE IF NOT EXISTS scan_cache (
    path     TEXT    PRIMARY KEY,
    mtime    INTEGER NOT NULL,
    size     INTEGER NOT NULL,
    category INTEGER NOT NULL
);`

// Entry holds the cached classification for a single file.
type Entry struct {
	Path     string
	Mtime    int64
	Size     int64
	Category classifier.Category
}

// Cache is a SQLite-backed store for file classification results.
// Upsert and Reset are protected by a mutex for safe concurrent use.
type Cache struct {
	db *sql.DB
	mu sync.Mutex
}

// Open opens (or creates) the SQLite database at path.
// Pass ":memory:" for an in-memory database suitable for tests.
func Open(path string) (*Cache, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

// Lookup returns the cached category for path when mtime and size match.
// Returns (category, true, nil) on hit; (0, false, nil) on miss.
func (c *Cache) Lookup(path string, mtime, size int64) (classifier.Category, bool, error) {
	var cat int
	err := c.db.QueryRow(
		`SELECT category FROM scan_cache WHERE path = ? AND mtime = ? AND size = ?`,
		path, mtime, size,
	).Scan(&cat)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return classifier.Category(cat), true, nil
}

// Upsert inserts or replaces a cache entry.
func (c *Cache) Upsert(e Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO scan_cache (path, mtime, size, category) VALUES (?, ?, ?, ?)`,
		e.Path, e.Mtime, e.Size, int(e.Category),
	)
	return err
}

// Reset removes all entries from the cache.
func (c *Cache) Reset() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`DELETE FROM scan_cache`)
	return err
}

// Close closes the underlying database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./cache/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cache/
git commit -m "feat: SQLite scan cache with concurrent-safe writes"
```

---

## Task 5: Scanner — directory walker and worker pool

**Files:**
- Modify: `scanner/scanner.go`
- Create: `scanner/scanner_test.go`

- [ ] **Step 1: Write failing tests**

Create `scanner/scanner_test.go`:
```go
package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ianchesal/itunes-detangler/classifier"
)

func TestScanClassifiesFiles(t *testing.T) {
	dir := t.TempDir()
	want := map[string]classifier.Category{
		"track.mp3":    classifier.CategoryDRMFree,
		"track.flac":   classifier.CategoryDRMFree,
		"track.m4p":    classifier.CategoryDRMProtected,
		"artwork.jpg":  classifier.CategorySkip,
		"sub/deep.mp3": classifier.CategoryDRMFree,
	}
	for name := range want {
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte{}, 0644)
	}

	s := &Scanner{Workers: 2, Cache: nil}
	results, err := s.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := map[string]classifier.Category{}
	for r := range results {
		if r.Err != nil {
			t.Errorf("result error for %s: %v", r.Path, r.Err)
			continue
		}
		rel, _ := filepath.Rel(dir, r.Path)
		got[rel] = r.Category
	}

	for name, wantCat := range want {
		if gotCat, ok := got[name]; !ok {
			t.Errorf("missing result for %s", name)
		} else if gotCat != wantCat {
			t.Errorf("%s: got %v, want %v", name, gotCat, wantCat)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d results, want %d", len(got), len(want))
	}
}

func TestScanCancellationDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 100; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("track%03d.mp3", i)), []byte{}, 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before scan starts

	s := &Scanner{Workers: 2, Cache: nil}
	results, err := s.Scan(ctx, dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for range results {} // must not hang
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./scanner/... -v
```

Expected: compile error — `Scanner` not defined.

- [ ] **Step 3: Implement the scanner**

Replace `scanner/scanner.go`:
```go
package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ianchesal/itunes-detangler/cache"
	"github.com/ianchesal/itunes-detangler/classifier"
)

// Result holds the classification result for a single file.
type Result struct {
	Path     string
	Mtime    int64
	Size     int64
	Category classifier.Category
	Err      error
}

// Scanner walks a directory tree and classifies files concurrently.
type Scanner struct {
	Workers int
	Cache   *cache.Cache // nil disables caching
}

// Scan walks root and sends one Result per file to the returned channel.
// The channel is closed when all files are processed or ctx is cancelled.
func (s *Scanner) Scan(ctx context.Context, root string) (<-chan Result, error) {
	results := make(chan Result, s.Workers*4)

	go func() {
		defer close(results)

		work := make(chan fileInfo, s.Workers*4)

		var wg sync.WaitGroup
		for i := 0; i < s.Workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for fi := range work {
					cat, err := s.classify(fi)
					select {
					case results <- Result{Path: fi.path, Mtime: fi.mtime, Size: fi.size, Category: cat, Err: err}:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			select {
			case <-ctx.Done():
				return fs.SkipAll
			default:
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			work <- fileInfo{
				path:  filepath.Join(root, path),
				mtime: info.ModTime().Unix(),
				size:  info.Size(),
			}
			return nil
		})

		close(work)
		wg.Wait()
	}()

	return results, nil
}

type fileInfo struct {
	path  string
	mtime int64
	size  int64
}

func (s *Scanner) classify(fi fileInfo) (classifier.Category, error) {
	if s.Cache != nil {
		if cat, ok, err := s.Cache.Lookup(fi.path, fi.mtime, fi.size); err == nil && ok {
			return cat, nil
		}
	}
	cat, err := classifier.Classify(fi.path)
	if err != nil {
		return classifier.CategorySkip, err
	}
	if s.Cache != nil {
		s.Cache.Upsert(cache.Entry{Path: fi.path, Mtime: fi.mtime, Size: fi.size, Category: cat})
	}
	return cat, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./scanner/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add scanner/
git commit -m "feat: concurrent directory scanner with worker pool"
```

---

## Task 6: Reporter — output files, progress bar, rsync command

**Files:**
- Modify: `reporter/reporter.go`
- Create: `reporter/reporter_test.go`

- [ ] **Step 1: Write failing tests**

Create `reporter/reporter_test.go`:
```go
package reporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianchesal/itunes-detangler/classifier"
	"github.com/ianchesal/itunes-detangler/scanner"
)

func TestReporterWritesOutputFiles(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{
		OutDir:   dir,
		SrcPath:  "/Fatboy/Musc/iTunes",
		DestPath: "/Volumes/media/Sorted",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rep.Record(scanner.Result{Path: "/Fatboy/Musc/iTunes/Phish/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/Fatboy/Musc/iTunes/Dead/show.m4p", Category: classifier.CategoryDRMProtected})
	rep.Record(scanner.Result{Path: "/Fatboy/Musc/iTunes/artwork.jpg", Category: classifier.CategorySkip})

	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}
	if stats.DRMFree != 1 {
		t.Errorf("DRMFree = %d, want 1", stats.DRMFree)
	}
	if stats.DRMProtected != 1 {
		t.Errorf("DRMProtected = %d, want 1", stats.DRMProtected)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}

	freeData, err := os.ReadFile(filepath.Join(dir, "drm-free.txt"))
	if err != nil {
		t.Fatalf("read drm-free.txt: %v", err)
	}
	if !strings.Contains(string(freeData), "Phish/track.mp3") {
		t.Errorf("drm-free.txt missing expected path; got: %q", string(freeData))
	}

	protData, err := os.ReadFile(filepath.Join(dir, "drm-protected.txt"))
	if err != nil {
		t.Fatalf("read drm-protected.txt: %v", err)
	}
	if !strings.Contains(string(protData), "Dead/show.m4p") {
		t.Errorf("drm-protected.txt missing expected path; got: %q", string(protData))
	}
}

func TestReporterDryRunSkipsFiles(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{
		OutDir:   dir,
		SrcPath:  "/src",
		DestPath: "/dst",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rep.Record(scanner.Result{Path: "/src/track.mp3", Category: classifier.CategoryDRMFree})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.DRMFree != 1 {
		t.Errorf("DRMFree = %d, want 1", stats.DRMFree)
	}
	if _, err := os.Stat(filepath.Join(dir, "drm-free.txt")); !os.IsNotExist(err) {
		t.Error("drm-free.txt should not exist in dry-run mode")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./reporter/... -v
```

Expected: compile error — `Reporter`, `New`, `Config`, `Stats` not defined.

- [ ] **Step 3: Implement the reporter**

Replace `reporter/reporter.go`:
```go
package reporter

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/schollz/progressbar/v3"

	"github.com/ianchesal/itunes-detangler/classifier"
	"github.com/ianchesal/itunes-detangler/scanner"
)

// Stats holds aggregate counts from a completed scan.
type Stats struct {
	Total        int
	DRMFree      int
	DRMProtected int
	Skipped      int
}

// Config configures a Reporter.
type Config struct {
	OutDir   string
	SrcPath  string
	DestPath string
	DryRun   bool
}

// Reporter writes classification results to output files and tracks statistics.
type Reporter struct {
	cfg        Config
	bar        *progressbar.ProgressBar
	freeWriter io.Writer
	protWriter io.Writer
	freeFile   *os.File
	protFile   *os.File
	stats      Stats
}

// New creates a Reporter. In non-dry-run mode, output files are created immediately.
func New(cfg Config) (*Reporter, error) {
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetDescription("Scanning"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
		progressbar.OptionClearOnFinish(),
	)
	r := &Reporter{cfg: cfg, bar: bar}

	if cfg.DryRun {
		r.freeWriter = io.Discard
		r.protWriter = io.Discard
		return r, nil
	}

	freeFile, err := os.Create(filepath.Join(cfg.OutDir, "drm-free.txt"))
	if err != nil {
		return nil, err
	}
	protFile, err := os.Create(filepath.Join(cfg.OutDir, "drm-protected.txt"))
	if err != nil {
		freeFile.Close()
		return nil, err
	}
	r.freeFile = freeFile
	r.protFile = protFile
	r.freeWriter = bufio.NewWriter(freeFile)
	r.protWriter = bufio.NewWriter(protFile)
	return r, nil
}

// Record counts and writes one scan result.
func (r *Reporter) Record(result scanner.Result) {
	r.bar.Add(1)
	r.stats.Total++

	rel, err := filepath.Rel(r.cfg.SrcPath, result.Path)
	if err != nil {
		rel = result.Path
	}

	switch result.Category {
	case classifier.CategoryDRMFree:
		r.stats.DRMFree++
		fmt.Fprintln(r.freeWriter, rel)
	case classifier.CategoryDRMProtected:
		r.stats.DRMProtected++
		fmt.Fprintln(r.protWriter, rel)
	default:
		r.stats.Skipped++
	}
}

// Finish flushes output, prints the summary and rsync command, and returns stats.
func (r *Reporter) Finish() (Stats, error) {
	if fw, ok := r.freeWriter.(*bufio.Writer); ok {
		if err := fw.Flush(); err != nil {
			return r.stats, err
		}
	}
	if pw, ok := r.protWriter.(*bufio.Writer); ok {
		if err := pw.Flush(); err != nil {
			return r.stats, err
		}
	}
	r.closeFiles()
	r.bar.Finish()

	fmt.Printf("\nScan complete: %d files | %d owned | %d DRM-protected | %d skipped\n",
		r.stats.Total, r.stats.DRMFree, r.stats.DRMProtected, r.stats.Skipped)

	if !r.cfg.DryRun {
		freePath, _ := filepath.Abs(filepath.Join(r.cfg.OutDir, "drm-free.txt"))
		fmt.Printf("\nrsync -av --files-from=%s %s %s\n", freePath, r.cfg.SrcPath, r.cfg.DestPath)
	}

	return r.stats, nil
}

// Close releases any open file handles. Safe to call after Finish.
func (r *Reporter) Close() {
	r.closeFiles()
}

func (r *Reporter) closeFiles() {
	if r.freeFile != nil {
		r.freeFile.Close()
		r.freeFile = nil
	}
	if r.protFile != nil {
		r.protFile.Close()
		r.protFile = nil
	}
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./reporter/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add reporter/
git commit -m "feat: reporter with buffered output, progress bar, and rsync command"
```

---

## Task 7: CLI — wire everything together

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Implement main.go**

Replace `main.go`:
```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ianchesal/itunes-detangler/cache"
	"github.com/ianchesal/itunes-detangler/reporter"
	"github.com/ianchesal/itunes-detangler/scanner"
)

const version = "0.1.0"

func main() {
	srcPath := flag.String("path", "/Fatboy/Musc/iTunes", "Source iTunes library path")
	destPath := flag.String("dest", "/Volumes/media/Sorted/Unsorted/iTunes", "rsync destination path")
	outDir := flag.String("out", ".", "Directory to write output files")
	workers := flag.Int("workers", 8, "Number of concurrent classifier workers")
	cachePath := flag.String("cache", defaultCachePath(), "Path to SQLite cache file")
	reset := flag.Bool("reset", false, "Wipe the cache and do a full rescan")
	dryRun := flag.Bool("dry-run", false, "Scan and classify but don't write output files")
	showVer := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Printf("itunes-detangler %s\n", version)
		return
	}

	c, err := cache.Open(*cachePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to open cache: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if *reset {
		if err := c.Reset(); err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to reset cache: %v\n", err)
			os.Exit(1)
		}
	}

	rep, err := reporter.New(reporter.Config{
		OutDir:   *outDir,
		SrcPath:  *srcPath,
		DestPath: *destPath,
		DryRun:   *dryRun,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create reporter: %v\n", err)
		os.Exit(1)
	}
	defer rep.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &scanner.Scanner{Workers: *workers, Cache: c}
	results, err := s.Scan(ctx, *srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: scan failed: %v\n", err)
		os.Exit(1)
	}

	for result := range results {
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", result.Path, result.Err)
			continue
		}
		rep.Record(result)
	}

	if _, err := rep.Finish(); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to finalize output: %v\n", err)
		os.Exit(1)
	}
}

func defaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".itunes-detangler-cache.db"
	}
	return filepath.Join(home, ".itunes-detangler", "cache.db")
}
```

- [ ] **Step 2: Build the binary**

```bash
go build -o itunes-detangler .
```

Expected: `itunes-detangler` binary created, no errors.

- [ ] **Step 3: Smoke test with a small temporary directory**

```bash
mkdir -p /tmp/itunes-test/Phish/2003
mkdir -p /tmp/itunes-test/Dead/Europe72
touch /tmp/itunes-test/Phish/2003/track01.mp3
touch /tmp/itunes-test/Phish/2003/track02.flac
touch /tmp/itunes-test/Dead/Europe72/track01.m4p
touch /tmp/itunes-test/artwork.jpg

./itunes-detangler --path /tmp/itunes-test --dest /tmp/nas --out /tmp --dry-run
```

Expected:
```
Scan complete: 4 files | 2 owned | 1 DRM-protected | 1 skipped
```

- [ ] **Step 4: Test --version flag**

```bash
./itunes-detangler --version
```

Expected: `itunes-detangler 0.1.0`

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all packages PASS, no failures.

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat: CLI entry point — itunes-detangler v0.1.0 complete"
```
