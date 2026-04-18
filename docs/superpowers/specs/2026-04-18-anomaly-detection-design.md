# Design: Anomaly Detection — Mixed-DRM Directories

**Date:** 2026-04-18
**Status:** Approved

## Overview

Add detection of directories that contain a mix of DRM-free and DRM-protected audio files. This is unexpected in a well-organized iTunes library (an album is normally all one or the other) and flags content worth investigating before migration.

Anomalies are surfaced two ways: printed to stdout at the end of the scan, and written to `anomalies.txt` in the `--out` directory.

---

## Scope

Single anomaly type: **mixed-DRM directory** — a directory whose direct children include at least one `CategoryDRMFree` file and at least one `CategoryDRMProtected` file.

- Detection is at the **direct parent directory level only**. A parent directory containing mixed-category *subdirectories* is not itself flagged.
- `CategorySkip` files (artwork, metadata, etc.) do not contribute to the determination.
- Files sitting directly under `--path` (relative dir `"."`) are grouped together and evaluated like any other directory.

---

## Changes

All changes are confined to `reporter/reporter.go` and `reporter/reporter_test.go`.

### Data model

Add a private `dirFlags` struct:

```go
type dirFlags struct {
    hasFree bool
    hasProt bool
}
```

Add a `dirState map[string]*dirFlags` field to `Reporter`, keyed on the **relative** path of each file's direct parent directory (i.e. `filepath.Dir(rel)` where `rel` is already computed in `Record()`).

Add `MixedDRMDirs int` to `Stats`.

### `Record()` changes

After computing `rel`, derive `dir := filepath.Dir(rel)`. In the existing `switch` on `result.Category`:

- `CategoryDRMFree` → ensure `dirState[dir]` exists, set `hasFree = true`
- `CategoryDRMProtected` → ensure `dirState[dir]` exists, set `hasProt = true`
- `CategorySkip` → no map update

### `Finish()` changes

After flushing and closing files, collect mixed directories:

1. Walk `dirState`, collect all keys where `hasFree && hasProt`.
2. Sort the slice for deterministic output.
3. Set `stats.MixedDRMDirs = len(mixed)`.

**stdout** — printed after the summary line, only if `len(mixed) > 0`:

```
Anomalies: 3 mixed-DRM directories
  Artist/Album Name
  Artist/Another Album
  Compilations/Mix
```

No output added to the summary line itself; the anomaly block is separate.

**`anomalies.txt`** — written to `--out` directory in non-dry-run mode always (even if empty), one relative path per line. Consistent with the always-written behavior of `drm-free.txt` and `drm-protected.txt`.

In dry-run mode: `anomalies.txt` is not written, but the stdout anomaly block is still printed if anomalies are found.

---

## Output Files

| File | Written in dry-run? | Content when no anomalies |
|---|---|---|
| `anomalies.txt` | No | Empty file |

---

## Testing

New test cases in `reporter_test.go` (table-driven, no new infrastructure):

| Scenario | Expected result |
|---|---|
| Directory with only DRM-free files | Not flagged |
| Directory with only DRM-protected files | Not flagged |
| Directory with both DRM-free and DRM-protected files | Flagged |
| Multiple directories, only some mixed | Only mixed ones appear |
| Skip files present in a mixed directory | Don't affect the result |
| Files at root (`dir == "."`) | Handled correctly |
| Dry-run mode with anomalies | Stdout block printed, no file written |

---

## Non-Goals

- No detection of other anomaly types (zero-byte files, duplicates, extension mismatches, etc.).
- No recursive / parent-directory rollup of mixed status.
- No changes to Scanner, Cache, Classifier, or CLI flags.
