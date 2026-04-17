package cache

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/ianchesal/itunes-detangler/classifier"
)

const schema = `
CREATE TABLE IF NOT EXISTS scan_cache (
    path     TEXT    PRIMARY KEY,
    mtime    INTEGER NOT NULL,
    size     INTEGER NOT NULL,
    category INTEGER NOT NULL
);`

// Entry holds the cached classification for a single file.
type Entry struct {
	Path     string
	Mtime    int64
	Size     int64
	Category classifier.Category
}

// Cache is a SQLite-backed store for file classification results.
// Upsert and Reset are protected by a mutex for safe concurrent use.
type Cache struct {
	db *sql.DB
	mu sync.Mutex
}

// Open opens (or creates) the SQLite database at path.
// Pass ":memory:" for an in-memory database suitable for tests.
func Open(path string) (*Cache, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

// Lookup returns the cached category for path when mtime and size match.
// Returns (category, true, nil) on hit; (0, false, nil) on miss.
func (c *Cache) Lookup(path string, mtime, size int64) (classifier.Category, bool, error) {
	var cat int
	err := c.db.QueryRow(
		`SELECT category FROM scan_cache WHERE path = ? AND mtime = ? AND size = ?`,
		path, mtime, size,
	).Scan(&cat)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return classifier.Category(cat), true, nil
}

// Upsert inserts or replaces a cache entry.
func (c *Cache) Upsert(e Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO scan_cache (path, mtime, size, category) VALUES (?, ?, ?, ?)`,
		e.Path, e.Mtime, e.Size, int(e.Category),
	)
	return err
}

// Reset removes all entries from the cache.
func (c *Cache) Reset() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`DELETE FROM scan_cache`)
	return err
}

// Close closes the underlying database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}
