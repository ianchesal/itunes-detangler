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

func TestReporterMixedDRMDirectory(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/Artist/Album/track1.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Artist/Album/track2.m4p", Category: classifier.CategoryDRMProtected})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if !strings.Contains(string(data), "Artist/Album") {
		t.Errorf("anomalies.txt missing expected dir; got: %q", string(data))
	}
}

func TestReporterNoMixedDRMDirectory(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/Free/track1.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Free/track2.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Protected/show.m4p", Category: classifier.CategoryDRMProtected})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 0 {
		t.Errorf("MixedDRMDirs = %d, want 0", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Errorf("anomalies.txt should be empty; got: %q", string(data))
	}
}

func TestReporterSkipFilesIgnoredForMixedDRM(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/Album/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Album/cover.jpg", Category: classifier.CategorySkip})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 0 {
		t.Errorf("MixedDRMDirs = %d, want 0 (skip files must not count)", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Errorf("anomalies.txt should be empty; got: %q", string(data))
	}
}

func TestReporterMixedDRMMultipleDirs(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/A/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/A/track.m4p", Category: classifier.CategoryDRMProtected})
	rep.Record(scanner.Result{Path: "/src/B/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/C/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/C/track.m4p", Category: classifier.CategoryDRMProtected})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 2 {
		t.Errorf("MixedDRMDirs = %d, want 2", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "A\n") {
		t.Errorf("anomalies.txt missing dir A; got: %q", content)
	}
	if !strings.Contains(content, "C\n") {
		t.Errorf("anomalies.txt missing dir C; got: %q", content)
	}
	if strings.Contains(content, "B\n") {
		t.Errorf("anomalies.txt must not contain dir B; got: %q", content)
	}
}

func TestReporterRootFilesAreMixed(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/track.m4p", Category: classifier.CategoryDRMProtected})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1 (root dir must be flagged)", stats.MixedDRMDirs)
	}
	data, err := os.ReadFile(filepath.Join(dir, "anomalies.txt"))
	if err != nil {
		t.Fatalf("read anomalies.txt: %v", err)
	}
	if !strings.Contains(string(data), ".\n") {
		t.Errorf("anomalies.txt missing root dir entry; got: %q", string(data))
	}
}

func TestReporterDryRunAnomaliesFileNotWritten(t *testing.T) {
	dir := t.TempDir()
	rep, err := New(Config{OutDir: dir, SrcPath: "/src", DestPath: "/dst", DryRun: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rep.Record(scanner.Result{Path: "/src/Album/track.mp3", Category: classifier.CategoryDRMFree})
	rep.Record(scanner.Result{Path: "/src/Album/track.m4p", Category: classifier.CategoryDRMProtected})
	stats, err := rep.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if stats.MixedDRMDirs != 1 {
		t.Errorf("MixedDRMDirs = %d, want 1", stats.MixedDRMDirs)
	}
	if _, err := os.Stat(filepath.Join(dir, "anomalies.txt")); !os.IsNotExist(err) {
		t.Error("anomalies.txt must not exist in dry-run mode")
	}
}
