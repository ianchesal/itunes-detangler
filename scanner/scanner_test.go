package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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
