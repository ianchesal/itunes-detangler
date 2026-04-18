# itunes-detangler

Scans an iTunes/Music.app library, identifies DRM-free tracks, and produces `rsync`-ready file lists for migrating music to a Plex server (or anywhere else).

Detects DRM without iTunes being open — reads MP4 box headers directly. `.mp3`, `.flac`, `.aiff`, `.wav` are always free; `.m4p` is always protected; `.m4a` files are inspected for FairPlay `sinf` boxes.

## Install

```sh
make install
```

## Usage

```
itunes-detangler [flags]

  --path     iTunes library path  (default: /Fatboy/Musc/iTunes)
  --dest     rsync destination    (default: /Volumes/media/Sorted/Unsorted/iTunes)
  --out      Output directory     (default: .)
  --workers  Worker count         (default: 8)
  --cache    SQLite cache path    (default: ~/.itunes-detangler/cache.db)
  --reset    Wipe cache, full rescan
  --dry-run  Classify only, no files written
  --version  Print version
```

Writes `drm-free.txt` and `drm-protected.txt` (paths relative to `--path`) and prints a ready-to-run `rsync` command.

## Make targets

| Target | Description |
|--------|-------------|
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make test-verbose` | Run tests with `-v` |
| `make test-race` | Run tests with race detector |
| `make lint` | Run `go vet` |
| `make install` | Install to `$GOPATH/bin` |
| `make clean` | Remove built binary |
