package classifier

import (
	"path/filepath"
	"strings"
)

// Category is the classification of a music file.
type Category int

const (
	CategorySkip         Category = iota // non-music, artwork, or unrecognised
	CategoryDRMFree                      // owned, freely copyable
	CategoryDRMProtected                 // owned but Fairplay-protected
)

func (c Category) String() string {
	switch c {
	case CategoryDRMFree:
		return "drm-free"
	case CategoryDRMProtected:
		return "drm-protected"
	default:
		return "skip"
	}
}

// Classify returns the category for the file at path.
// .m4a files are inspected via MP4 box headers; all other formats are
// determined by extension alone.
func Classify(path string) (Category, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac", ".aiff", ".aif", ".wav":
		return CategoryDRMFree, nil
	case ".m4p":
		return CategoryDRMProtected, nil
	case ".m4a":
		return classifyM4A(path)
	default:
		return CategorySkip, nil
	}
}
