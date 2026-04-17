# Design: itunes-detangler

**Date:** 2026-04-17
**Status:** Approved

## Overview

A macOS CLI tool written in Go that scans an iTunes/Music.app library on disk, identifies DRM-free tracks the user owns, and produces output files suitable for use with `rsync` to migrate music to a Plex server on a NAS.

The tool must handle libraries up to ~1TB (hundreds of thousands of files) efficiently using concurrent scanning and a SQLite-backed scan cache for fast incremental runs.

---

## Detection Strategy

Track classification is done entirely at the file level — no dependency on iTunes XML export (removed in recent macOS versions) or Music.app being open.

### Categories

| Category | Description |
|---|---|
| **DRM-free** | Tracks the user owns and can copy freely |
| **DRM-protected** | Tracks the user owns but are Fairplay-protected (`.m4p` or `.m4a` with `sinf` box) |
| **Skip** | Streaming cache, artwork, metadata files — ignored |

### Classification Rules

- `.mp3`, `.flac`, `.aiff`, `.aif`, `.wav` → **DRM-free** (these formats cannot carry DRM)
- `.m4p` → **DRM-protected** (extension is definitive, no header inspection needed)
- `.m4a` → inspect MP4 box tree:
  - Sample entry type `alac` → **DRM-free** (Apple Lossless)
  - Sample entry type `mp4a` without `sinf` child box → **DRM-free** (AAC)
  - Sample entry type `mp4a` with `sinf` child box → **DRM-protected** (Fairplay AAC)
- All other extensions → **Skip**

### MP4 DRM Detection

Reads only the first few KB of `.m4a` files — enough to traverse the MP4 box hierarchy to `moov → trak → mdia → minf → stbl → stsd` without loading audio data. Uses only Go stdlib `encoding/binary`. Stops reading as soon as the determination is made.

---

## Architecture

### Components

1. **CLI layer** (`main.go`) — parses flags, wires components, handles Ctrl-C graceful shutdown
2. **Scanner** (`scanner/`) — walks the source directory tree, feeds file paths to workers via a buffered channel
3. **Classifier** (`classifier/`) — determines a file's category from its extension and MP4 box headers
4. **Cache** (`cache/`) — SQLite database storing `(path, mtime, size, category)`; unchanged files skip the classifier
5. **Reporter** (`reporter/`) — writes output files, prints the rsync command, drives the progress bar
6. **Progress bar** — terminal progress bar showing files/sec, total scanned, per-category counts

### Data Flow

```
source dir
    │
    ▼
Scanner (directory walker)
    │  file paths (buffered channel)
    ▼
Worker pool (--workers, default 8)
    │
    ├── Cache lookup (SQLite)
    │       ├── hit (mtime+size unchanged) → use cached category
    │       └── miss / changed → Classifier → Cache upsert
    │
    ▼
Aggregator (single goroutine)
    ├── drm-free.txt
    ├── drm-protected.txt
    └── progress bar + final rsync command (stdout)
```

The aggregator is a single goroutine collecting results from all workers, eliminating write contention on output files.

On Ctrl-C: scanner stops walking, in-flight workers complete their current file, output files are flushed and closed.

---

## Project Structure

```
itunes-detangler/
├── main.go
├── go.mod
├── go.sum
├── .gitignore
├── DESIGN.md
├── classifier/
│   ├── classifier.go   # Category type, Classify(path) function
│   └── mp4.go          # MP4 box parser for DRM detection
├── scanner/
│   └── scanner.go      # Directory walker + worker pool
├── cache/
│   └── cache.go        # SQLite open/lookup/upsert/reset
├── reporter/
│   └── reporter.go     # Output file writer + rsync command printer
└── docs/
    └── superpowers/
        └── specs/
            └── 2026-04-17-itunes-detangler-design.md
```

---

## CLI Interface

```
itunes-detangler [flags]

Flags:
  --path      Source iTunes library path
              (default: /Fatboy/Musc/iTunes)
  --dest      rsync destination path
              (default: /Volumes/media/Sorted/Unsorted/iTunes)
  --out       Directory to write output files
              (default: current working directory)
  --workers   Number of concurrent classifier workers
              (default: 8)
  --cache     Path to SQLite cache file
              (default: ~/.itunes-detangler/cache.db)
  --reset     Wipe the cache and do a full rescan
  --dry-run   Scan and classify but don't write output files
  --version   Print version and exit
```

### Output Files

Written to `--out` directory:
- `drm-free.txt` — one path per line, relative to `--path`, for use with `rsync --files-from`
- `drm-protected.txt` — one path per line, relative to `--path`, for manual review

### Final stdout on completion

```
Scan complete: 142,847 files | 98,432 owned | 1,204 DRM-protected | 43,211 skipped

rsync -av --files-from=/path/to/out/drm-free.txt /Fatboy/Musc/iTunes /Volumes/media/Sorted/Unsorted/iTunes
```

The rsync command uses the full path to `drm-free.txt` based on the resolved `--out` value.

---

## Dependencies

| Package | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite driver (no CGo, no build complexity) |
| `schollz/progressbar/v3` | Terminal progress bar |

All other functionality uses Go stdlib only.

---

## .gitignore

```
itunes-detangler    # compiled binary
*.txt               # output file lists (drm-free.txt, drm-protected.txt)
*.db                # SQLite cache database
```

---

## Key Constraints

- Must handle ~1TB / hundreds of thousands of files efficiently
- No dependency on Music.app being open or iTunes XML export
- Graceful Ctrl-C shutdown — no partial/corrupt output files
- Pure-Go build (no CGo) for simplicity
- Single static binary output
