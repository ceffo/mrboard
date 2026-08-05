// Package diskstore implements github.com/eko/gocache/lib/v4/store.StoreInterface
// on top of an afero.Fs, so callers can inject an in-memory filesystem in tests
// instead of touching real disk. It knows nothing about the shape of cached
// values — gocache's marshaler owns that — it only persists opaque bytes with
// an expiry.
package diskstore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eko/gocache/lib/v4/store"
	"github.com/spf13/afero"
)

// Type identifies this store to gocache callers (StoreInterface.GetType).
const Type = "diskstore"

const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Clock supplies the current time. Production code gets realClock via New;
// tests inject a fake to control expiry deterministically instead of calling
// time.Now() (and sleeping, or fudging negative TTLs) to observe it.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Config holds Store's constructor arguments.
type Config struct {
	// FS is the filesystem entries are written through. Production callers
	// pass afero.NewOsFs(); tests pass afero.NewMemMapFs() to avoid touching disk.
	FS afero.Fs
	// Dir is the directory cache entries are written to, created if absent.
	Dir string
	// Clock supplies the current time; defaults to the real wall clock when nil.
	Clock Clock
}

// Store is a store.StoreInterface backed by one file per key under dir. Each
// file holds an 8-byte little-endian UnixNano expiry followed by the raw
// cached bytes.
type Store struct {
	fs    afero.Fs
	dir   string
	clock Clock
}

// New creates a Store rooted at cfg.Dir, creating the directory if absent.
func New(cfg Config) (*Store, error) {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if err := cfg.FS.MkdirAll(cfg.Dir, dirMode); err != nil {
		return nil, fmt.Errorf("diskstore: create dir %q: %w", cfg.Dir, err)
	}
	return &Store{fs: cfg.FS, dir: cfg.Dir, clock: cfg.Clock}, nil
}

// GetType implements store.StoreInterface.
func (s *Store) GetType() string { return Type }

// path derives a collision-free filename from key by hashing its string form —
// naive character substitution (e.g. "/" and ":" both to "_") can map two
// distinct keys to the same file, which a hash cannot.
func (s *Store) path(key any) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v", key)))
	return filepath.Join(s.dir, fmt.Sprintf("%x.cache", sum))
}

// Get implements store.StoreInterface.
func (s *Store) Get(ctx context.Context, key any) (any, error) {
	value, _, err := s.GetWithTTL(ctx, key)
	return value, err
}

// GetWithTTL implements store.StoreInterface. An absent, corrupt, or expired
// entry is always reported as a store.NotFound error — callers treat all
// three the same way: fall back to a live fetch.
func (s *Store) GetWithTTL(_ context.Context, key any) (any, time.Duration, error) {
	f, err := s.fs.Open(s.path(key))
	if err != nil {
		return nil, 0, store.NotFoundWithCause(err)
	}
	defer f.Close()

	var expiresAtUnixNano int64
	if err := binary.Read(f, binary.LittleEndian, &expiresAtUnixNano); err != nil {
		return nil, 0, store.NotFoundWithCause(err)
	}
	ttl := time.Unix(0, expiresAtUnixNano).Sub(s.clock.Now())
	if ttl <= 0 {
		return nil, 0, store.NotFoundWithCause(errors.New("diskstore: entry expired"))
	}

	value, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, store.NotFoundWithCause(err)
	}
	return value, ttl, nil
}

// Set implements store.StoreInterface. value must be []byte or string —
// gocache's marshaler always hands the store already-serialized bytes.
// An expiration of zero or less is written as already-expired.
func (s *Store) Set(_ context.Context, key any, value any, options ...store.Option) error {
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("diskstore: unsupported value type %T", value)
	}

	opts := store.ApplyOptions(options...)
	expiresAt := s.clock.Now().Add(opts.Expiration)

	path := s.path(key)
	f, err := s.fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("diskstore: create %q: %w", path, err)
	}
	defer f.Close()

	if err := binary.Write(f, binary.LittleEndian, expiresAt.UnixNano()); err != nil {
		return fmt.Errorf("diskstore: write expiry: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("diskstore: write value: %w", err)
	}
	return nil
}

// Delete implements store.StoreInterface. Deleting an absent key is not an error.
func (s *Store) Delete(_ context.Context, key any) error {
	path := s.path(key)
	if err := s.fs.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("diskstore: delete %q: %w", path, err)
	}
	return nil
}

// Invalidate implements store.StoreInterface. This store has no tag index, so
// tag-scoped invalidation is a no-op; untagged Invalidate calls are too.
func (s *Store) Invalidate(_ context.Context, _ ...store.InvalidateOption) error {
	return nil
}

// Clear implements store.StoreInterface, removing every entry under dir.
func (s *Store) Clear(_ context.Context) error {
	entries, err := afero.ReadDir(s.fs, s.dir)
	if err != nil {
		return fmt.Errorf("diskstore: read dir %q: %w", s.dir, err)
	}
	for _, entry := range entries {
		path := filepath.Join(s.dir, entry.Name())
		if err := s.fs.Remove(path); err != nil {
			return fmt.Errorf("diskstore: remove %q: %w", path, err)
		}
	}
	return nil
}

var _ store.StoreInterface = (*Store)(nil)
