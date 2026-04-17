package classifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyDRMFreeExtensions(t *testing.T) {
	for _, ext := range []string{".mp3", ".flac", ".aiff", ".aif", ".wav"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("Classify: unexpected error: %v", err)
			}
			if got != CategoryDRMFree {
				t.Errorf("Classify(%q) = %v, want %v", filepath.Ext(f.Name()), got, CategoryDRMFree)
			}
		})
	}
}

func TestClassifyM4PIsProtected(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.m4p")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMProtected {
		t.Errorf("Classify(.m4p) = %v, want CategoryDRMProtected", got)
	}
}

func TestClassifyUnknownExtensionIsSkip(t *testing.T) {
	for _, ext := range []string{".jpg", ".pdf", ".xml", ".nfo"} {
		t.Run(ext, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "*"+ext)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			got, err := Classify(f.Name())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != CategorySkip {
				t.Errorf("Classify(%q) = %v, want CategorySkip", ext, got)
			}
		})
	}
}

func TestClassifyIsCaseInsensitive(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.MP3")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Classify(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != CategoryDRMFree {
		t.Errorf("Classify(.MP3) = %v, want CategoryDRMFree", got)
	}
}
