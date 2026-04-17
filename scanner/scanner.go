package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ianchesal/itunes-detangler/cache"
	"github.com/ianchesal/itunes-detangler/classifier"
)

// Result holds the classification result for a single file.
type Result struct {
	Path     string
	Mtime    int64
	Size     int64
	Category classifier.Category
	Err      error
}

// Scanner walks a directory tree and classifies files concurrently.
type Scanner struct {
	Workers int
	Cache   *cache.Cache // nil disables caching
}

// Scan walks root and sends one Result per file to the returned channel.
// The channel is closed when all files are processed or ctx is cancelled.
// Returns an error immediately if root does not exist or is inaccessible.
func (s *Scanner) Scan(ctx context.Context, root string) (<-chan Result, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}

	results := make(chan Result, s.Workers*4)

	go func() {
		defer close(results)

		work := make(chan fileInfo, s.Workers*4)

		var wg sync.WaitGroup
		for i := 0; i < s.Workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for fi := range work {
					cat, err := s.classify(fi)
					select {
					case results <- Result{Path: fi.path, Mtime: fi.mtime, Size: fi.size, Category: cat, Err: err}:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Surface subdirectory access errors so the caller can warn the user.
				if path != "." {
					select {
					case results <- Result{Path: filepath.Join(root, path), Err: err}:
					case <-ctx.Done():
						return fs.SkipAll
					}
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			select {
			case <-ctx.Done():
				return fs.SkipAll
			default:
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			select {
			case work <- fileInfo{
				path:  filepath.Join(root, path),
				mtime: info.ModTime().Unix(),
				size:  info.Size(),
			}:
			case <-ctx.Done():
				return fs.SkipAll
			}
			return nil
		})

		close(work)
		wg.Wait()
	}()

	return results, nil
}

type fileInfo struct {
	path  string
	mtime int64
	size  int64
}

func (s *Scanner) classify(fi fileInfo) (classifier.Category, error) {
	if s.Cache != nil {
		if cat, ok, err := s.Cache.Lookup(fi.path, fi.mtime, fi.size); err == nil && ok {
			return cat, nil
		}
	}
	cat, err := classifier.Classify(fi.path)
	if err != nil {
		return classifier.CategorySkip, err
	}
	if s.Cache != nil {
		// Cache write failure is non-fatal: the file was classified correctly.
		// A miss on the next run just re-classifies at the cost of performance.
		_ = s.Cache.Upsert(cache.Entry{Path: fi.path, Mtime: fi.mtime, Size: fi.size, Category: cat})
	}
	return cat, nil
}
