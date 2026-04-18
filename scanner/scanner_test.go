package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ianchesal/itunes-detangler/cache"
	"github.com/ianchesal/itunes-detangler/classifier"
)

func TestScanClassifiesFiles(t *testing.T) {
	dir := t.TempDir()
	want := map[string]classifier.Category{
		"track.mp3":    classifier.CategoryDRMFree,
		"track.flac":   classifier.CategoryDRMFree,
		"track.m4p":    classifier.CategoryDRMProtected,
		"artwork.jpg":  classifier.CategorySkip,
		"sub/deep.mp3": classifier.CategoryDRMFree,
	}
	for name := range want {
		full := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte{}, 0644)
	}

	s := &Scanner{Workers: 2, Cache: nil}
	results, err := s.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := map[string]classifier.Category{}
	for r := range results {
		if r.Err != nil {
			t.Errorf("result error for %s: %v", r.Path, r.Err)
			continue
		}
		rel, _ := filepath.Rel(dir, r.Path)
		got[rel] = r.Category
	}

	for name, wantCat := range want {
		if gotCat, ok := got[name]; !ok {
			t.Errorf("missing result for %s", name)
		} else if gotCat != wantCat {
			t.Errorf("%s: got %v, want %v", name, gotCat, wantCat)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d results, want %d", len(got), len(want))
	}
}

func TestScanMissingRootReturnsError(t *testing.T) {
	s := &Scanner{Workers: 2, Cache: nil}
	_, err := s.Scan(context.Background(), "/does/not/exist")
	if err == nil {
		t.Error("expected error for non-existent root, got nil")
	}
}

func TestScanPreCancelDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 100; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("track%03d.mp3", i)), []byte{}, 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before scan starts

	s := &Scanner{Workers: 2, Cache: nil}
	results, err := s.Scan(ctx, dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for range results {} // must not hang
}

func TestScanCacheHitSkipsClassify(t *testing.T) {
	dir := t.TempDir()
	// A .xyz file has no classifier — Classify returns CategorySkip.
	// Pre-seed the cache with CategoryDRMFree so we can verify the cache is used.
	filePath := filepath.Join(dir, "track.xyz")
	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}

	c, err := cache.Open(":memory:")
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer c.Close()

	if err := c.Upsert(cache.Entry{
		Path:     filePath,
		Mtime:    info.ModTime().Unix(),
		Size:     info.Size(),
		Category: classifier.CategoryDRMFree,
	}); err != nil {
		t.Fatalf("cache.Upsert: %v", err)
	}

	s := &Scanner{Workers: 1, Cache: c}
	results, err := s.Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var got []Result
	for r := range results {
		got = append(got, r)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Err != nil {
		t.Fatalf("unexpected error: %v", got[0].Err)
	}
	if got[0].Category != classifier.CategoryDRMFree {
		t.Errorf("got category %v, want CategoryDRMFree (cache hit)", got[0].Category)
	}
}

func TestScanMidCancelDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 500; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("track%03d.mp3", i)), []byte{}, 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Scanner{Workers: 2, Cache: nil}
	results, err := s.Scan(ctx, dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Cancel after receiving the first result, while the scan is still in flight.
	// The channel must drain without hanging regardless of how many results remain.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range results {
			cancel()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not drain after mid-scan cancellation")
	}
}
