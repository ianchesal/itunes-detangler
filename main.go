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
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
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
		return nil
	}

	c, err := cache.Open(*cachePath)
	if err != nil {
		return fmt.Errorf("failed to open cache: %w", err)
	}
	defer c.Close()

	if *reset {
		if err := c.Reset(); err != nil {
			return fmt.Errorf("failed to reset cache: %w", err)
		}
	}

	rep, err := reporter.New(reporter.Config{
		OutDir:   *outDir,
		SrcPath:  *srcPath,
		DestPath: *destPath,
		DryRun:   *dryRun,
	})
	if err != nil {
		return fmt.Errorf("failed to create reporter: %w", err)
	}
	defer rep.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &scanner.Scanner{Workers: *workers, Cache: c}
	results, err := s.Scan(ctx, *srcPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
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
		return fmt.Errorf("failed to finalize output: %w", err)
	}
	return nil
}

func defaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".itunes-detangler-cache.db"
	}
	return filepath.Join(home, ".itunes-detangler", "cache.db")
}
