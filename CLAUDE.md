# itunes-detangler

A macOS CLI tool in Go that scans an iTunes/Music.app library, identifies DRM-free tracks you own, and produces rsync-ready file lists for migrating music to a Plex server on a NAS.

## Project Status

**v0.1.0 implemented and merged to main.**

- Spec: `docs/superpowers/specs/2026-04-17-itunes-detangler-design.md`
- Plan: `docs/superpowers/plans/2026-04-17-itunes-detangler.md`

## Context

Ian wants to move his owned music (Phish shows, Grateful Dead shows, etc.) out of iTunes and into Plex. The Music.app library lives at `/Fatboy/Musc/iTunes`. The NAS destination is `/Volumes/media/Sorted/Unsorted/iTunes`.

Recent macOS versions removed the "Share iTunes Library XML" setting, so the tool detects DRM by reading MP4 box headers directly — no dependency on Music.app being open or any XML export.

## Key Design Decisions

- **Go CLI**, single static binary, pure-Go dependencies (no CGo)
- **File-only classification** — no iTunes XML needed:
  - `.mp3`, `.flac`, `.aiff`, `.aif`, `.wav` → DRM-free
  - `.m4p` → DRM-protected (always)
  - `.m4a` → inspect MP4 `moov→trak→mdia→minf→stbl→stsd` box tree; presence of `sinf` box inside `mp4a` entry = Fairplay protected; `alac` codec = DRM-free
- **SQLite scan cache** (`modernc.org/sqlite`, pure-Go) — stores `(path, mtime, size, category)` for fast incremental re-runs on a ~1TB library
- **Output**: `drm-free.txt` and `drm-protected.txt` (paths relative to `--path`), plus a printed `rsync` command
- **Progress bar**: `schollz/progressbar/v3`, indeterminate (no pre-count pass needed)
- **Graceful Ctrl-C**: scanner stops walking, in-flight workers finish, files flushed cleanly

## CLI

```
itunes-detangler [flags]

  --path     Source iTunes library path  (default: /Fatboy/Musc/iTunes)
  --dest     rsync destination           (default: /Volumes/media/Sorted/Unsorted/iTunes)
  --out      Output directory            (default: .)
  --workers  Classifier worker count     (default: 8)
  --cache    SQLite cache path           (default: ~/.itunes-detangler/cache.db)
  --reset    Wipe cache, full rescan
  --dry-run  Classify only, no files written
  --version  Print version
```

## Package Layout

```
classifier/   Category type, Classify(path), MP4 box parser
cache/        SQLite open/lookup/upsert/reset
scanner/      Directory walker + worker pool
reporter/     Output files + progress bar + rsync command
main.go       CLI wiring
```

## To Resume Implementation

Open Claude Code in this directory and say:

> "Let's implement the plan at docs/superpowers/plans/2026-04-17-itunes-detangler.md"

The plan uses TDD with one task per package. Start at Task 1.
