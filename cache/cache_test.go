package cache

import (
	"testing"

	"github.com/ianchesal/itunes-detangler/classifier"
)

func openMemory(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestLookupMiss(t *testing.T) {
	c := openMemory(t)
	_, ok, err := c.Lookup("/some/path.mp3", 1000, 512)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on empty cache, got hit")
	}
}

func TestUpsertAndLookupHit(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/music/track.mp3", Mtime: 1700000000, Size: 4096000, Category: classifier.CategoryDRMFree}
	if err := c.Upsert(e); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, ok, err := c.Lookup(e.Path, e.Mtime, e.Size)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok {
		t.Fatal("expected hit, got miss")
	}
	if got != classifier.CategoryDRMFree {
		t.Errorf("got %v, want CategoryDRMFree", got)
	}
}

func TestLookupMissOnMtimeChange(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.mp3", Mtime: 100, Size: 500, Category: classifier.CategoryDRMFree}
	c.Upsert(e)
	_, ok, err := c.Lookup(e.Path, 999, e.Size) // different mtime
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on mtime change, got hit")
	}
}

func TestLookupMissOnSizeChange(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.mp3", Mtime: 100, Size: 500, Category: classifier.CategoryDRMFree}
	c.Upsert(e)
	_, ok, err := c.Lookup(e.Path, e.Mtime, 9999) // different size
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected miss on size change, got hit")
	}
}

func TestReset(t *testing.T) {
	c := openMemory(t)
	c.Upsert(Entry{Path: "/a.mp3", Mtime: 1, Size: 1, Category: classifier.CategoryDRMFree})
	c.Upsert(Entry{Path: "/b.mp3", Mtime: 1, Size: 1, Category: classifier.CategoryDRMFree})
	if err := c.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	_, ok, _ := c.Lookup("/a.mp3", 1, 1)
	if ok {
		t.Error("expected empty cache after Reset, got hit")
	}
}

func TestUpsertReplaces(t *testing.T) {
	c := openMemory(t)
	e := Entry{Path: "/track.m4a", Mtime: 100, Size: 500, Category: classifier.CategorySkip}
	c.Upsert(e)
	e.Category = classifier.CategoryDRMFree
	c.Upsert(e)
	got, ok, err := c.Lookup(e.Path, e.Mtime, e.Size)
	if err != nil || !ok {
		t.Fatalf("Lookup after upsert: err=%v ok=%v", err, ok)
	}
	if got != classifier.CategoryDRMFree {
		t.Errorf("got %v after replace upsert, want CategoryDRMFree", got)
	}
}
