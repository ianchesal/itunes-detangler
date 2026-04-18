# Go Code Review — itunes-detangler

Reviewed by: Claude (expert Go reviewer)
Date: 2026-04-18
Version: v0.1.0

---

## Overall Assessment

The code is well-structured, idiomatic Go. Package separation is clean (`classifier`, `cache`, `scanner`, `reporter`), the concurrency model is sound, and the MP4 box-parsing logic is correct and well-commented. Test coverage is good for the core paths.

Five issues are worth fixing before this is considered production-quality.

---

## Issues

### 1. SQLite concurrent writes without a connection limit (HIGH)

**File:** `cache/cache.go:38-53`

The `*sql.DB` pool allows multiple concurrent connections to the SQLite file. The scanner spawns up to 8 worker goroutines (default), all of which call `s.Cache.Upsert()` concurrently. Without WAL mode or a single-connection limit, SQLite's exclusive write lock causes concurrent writes to collide with `SQLITE_BUSY` errors.

The code silently discards cache write errors:

```go
// scanner/scanner.go:122
_ = s.Cache.Upsert(cache.Entry{...})
```

This means under normal load with 8 workers, the cache is populated unreliably. On a ~1TB library re-scan this defeats the purpose of the cache.

**Fix:** Add one of the following to `cache.Open` after `sql.Open`:

```go
// Option A: single connection (simplest, zero contention)
db.SetMaxOpenConns(1)

// Option B: WAL mode (higher throughput, multiple readers)
db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;")
```

Option A is the right default for this workload (write-heavy during scan, read-only during lookup).

---

### 2. `--workers=0` causes a deadlock (HIGH)

**File:** `scanner/scanner.go:37,44-57`

```go
results := make(chan Result, s.Workers*4)  // capacity 0
work    := make(chan fileInfo, s.Workers*4) // capacity 0

for i := 0; i < s.Workers; i++ { // no iterations
    ...
}

// Walker tries to send to unbuffered work with no receivers → blocks forever
fs.WalkDir(...)
```

If the user passes `--workers=0`, no worker goroutines are started, and the walk goroutine blocks trying to send to the unbuffered `work` channel. The main goroutine is blocked on `for result := range results`. Neither side can proceed. The process hangs silently.

**Fix:** Validate `Workers >= 1` in `Scan`, or in `main.go` before constructing the `Scanner`:

```go
if *workers < 1 {
    fmt.Fprintln(os.Stderr, "error: --workers must be >= 1")
    os.Exit(1)
}
```

---

### 3. `os.Exit` bypasses defers; output files left open on early failure (MEDIUM)

**File:** `main.go:42-47, 57-59, 68-70`

`defer c.Close()` and `defer rep.Close()` are registered before the early-exit error paths:

```go
defer c.Close()

if *reset {
    if err := c.Reset(); err != nil {
        os.Exit(1)  // defers do NOT run
    }
}

rep, err := reporter.New(...)
defer rep.Close()

results, err := s.Scan(...)
if err != nil {
    os.Exit(1)  // defers do NOT run
}
```

`os.Exit` does not run deferred functions. When `reporter.New` has already created `drm-free.txt` and `drm-protected.txt` (even empty), and then `s.Scan` fails, those files are never closed by `rep.Close()`. The OS closes them on process exit so data isn't corrupted, but SQLite's WAL may not be checkpointed cleanly.

**Fix:** Replace `os.Exit` with explicit cleanup, or factor the body of `main` into a `run() error` function that returns an error (the idiomatic Go pattern):

```go
func main() {
    if err := run(); err != nil {
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }
}

func run() error {
    // defers work correctly here
    c, err := cache.Open(...)
    if err != nil { return fmt.Errorf("open cache: %w", err) }
    defer c.Close()
    ...
}
```

---

### 4. `d.Info()` errors silently dropped in walker (MEDIUM)

**File:** `scanner/scanner.go:80-84`

```go
info, err := d.Info()
if err != nil {
    return nil  // file silently skipped, no Result sent
}
```

Directory-level access errors (a few lines above, L62-69) are correctly surfaced as `Result{Err: err}` entries so the caller can warn the user. Files that fail `d.Info()` (e.g., dangling symlinks, files deleted mid-walk) are silently dropped with no notification.

**Fix:** Send an error result for consistency:

```go
info, err := d.Info()
if err != nil {
    select {
    case results <- Result{Path: filepath.Join(root, path), Err: err}:
    case <-ctx.Done():
        return fs.SkipAll
    }
    return nil
}
```

---

### 5. No cache integration test in the scanner (LOW)

**File:** `scanner/scanner_test.go`

All scanner tests use `Cache: nil`. The cache hit/miss path in `Scanner.classify` (which is core to the tool's performance claim on large libraries) is never exercised:

```go
func (s *Scanner) classify(fi fileInfo) (classifier.Category, error) {
    if s.Cache != nil {
        if cat, ok, err := s.Cache.Lookup(...); err == nil && ok {
            return cat, nil  // ← never tested
        }
    }
    ...
    _ = s.Cache.Upsert(...)  // ← never tested
}
```

**Fix:** Add a test that opens a `:memory:` cache, pre-seeds an entry via `Upsert`, then scans and verifies the cached category is returned (and that `Classify` is not called — verifiable via a spy or a file without a valid extension that would otherwise return `CategorySkip`).

---

## Non-issues Worth Noting

These looked worth double-checking but are correct as written:

- **`findBox` `consumed` accounting**: correctly counts full box sizes (header + content) within the parent's content region. The seek arithmetic is right.
- **`os.DirFS` + `filepath.Join(root, path)`**: roundabout but correct; paths from the `fs.FS` walker are re-joined with `root` to produce absolute paths.
- **`bufio.Writer` type assertion in `Finish`**: `if fw, ok := r.freeWriter.(*bufio.Writer)` is fragile but safe given the controlled construction in `New`.
- **`shellQuote`**: correct POSIX single-quote escaping.
- **Concurrency model in `Scanner.Scan`**: goroutine fan-out with `sync.WaitGroup`, cancel-aware on both the `work` send and the `results` send. Clean, no deadlock under normal and cancelled operation.
- **`Close` after `Finish` double-call safety**: `closeFiles` nil-checks before closing; idempotent.

---

## Summary Table

| # | File | Severity | Description |
|---|------|----------|-------------|
| 1 | `cache/cache.go` | HIGH | SQLite concurrent writes without `SetMaxOpenConns(1)` or WAL |
| 2 | `scanner/scanner.go` | HIGH | `--workers=0` deadlock |
| 3 | `main.go` | MEDIUM | `os.Exit` bypasses defers; files left open |
| 4 | `scanner/scanner.go` | MEDIUM | `d.Info()` errors silently dropped |
| 5 | `scanner/scanner_test.go` | LOW | No cache integration test |
