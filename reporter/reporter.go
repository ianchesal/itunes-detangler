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
