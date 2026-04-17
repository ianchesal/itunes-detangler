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
