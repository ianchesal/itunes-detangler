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
			rep.RecordError()
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
