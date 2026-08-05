package diskstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/gocache/lib/v4/store"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(Config{FS: afero.NewMemMapFs(), Dir: "/cache"})
	require.NoError(t, err)
	return s
}

// fakeClock is a Clock whose Now() is set explicitly, so expiry can be
// exercised deterministically instead of via real sleeps or negative TTLs.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestSetGet_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("hello"), store.WithExpiration(time.Hour)))

	value, err := s.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), value)
}

func TestGet_Miss(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Get(context.Background(), "absent")
	require.Error(t, err)
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestGetWithTTL_ReturnsRemainingDuration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v"), store.WithExpiration(time.Hour)))

	_, ttl, err := s.GetWithTTL(ctx, "k1")
	require.NoError(t, err)
	assert.Positive(t, ttl)
	assert.LessOrEqual(t, ttl, time.Hour)
}

func TestGet_ExpiredEntryIsANotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v"), store.WithExpiration(-time.Second)))

	_, err := s.Get(ctx, "k1")
	require.Error(t, err)
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestSet_ZeroExpirationIsImmediatelyExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v")))

	_, err := s.Get(ctx, "k1")
	require.Error(t, err)
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestSet_AcceptsStringValue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", "a string value", store.WithExpiration(time.Hour)))

	value, err := s.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("a string value"), value)
}

func TestSet_RejectsUnsupportedValueType(t *testing.T) {
	s := newTestStore(t)

	err := s.Set(context.Background(), "k1", 42, store.WithExpiration(time.Hour))
	require.Error(t, err)
}

func TestSet_OverwritesExistingEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("first"), store.WithExpiration(time.Hour)))
	require.NoError(t, s.Set(ctx, "k1", []byte("second"), store.WithExpiration(time.Hour)))

	value, err := s.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("second"), value)
}

func TestDelete_RemovesEntry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v"), store.WithExpiration(time.Hour)))
	require.NoError(t, s.Delete(ctx, "k1"))

	_, err := s.Get(ctx, "k1")
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestDelete_AbsentKeyIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Delete(context.Background(), "never-existed"))
}

func TestClear_RemovesAllEntriesButKeepsOthersUnaffected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v1"), store.WithExpiration(time.Hour)))
	require.NoError(t, s.Set(ctx, "k2", []byte("v2"), store.WithExpiration(time.Hour)))

	require.NoError(t, s.Clear(ctx))

	_, err := s.Get(ctx, "k1")
	assert.ErrorIs(t, err, &store.NotFound{})
	_, err = s.Get(ctx, "k2")
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestKeysWithUnsafeCharactersDoNotCollideOrError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "OD:3345", []byte("colon"), store.WithExpiration(time.Hour)))
	require.NoError(t, s.Set(ctx, "OD/3345", []byte("slash"), store.WithExpiration(time.Hour)))

	v1, err := s.Get(ctx, "OD:3345")
	require.NoError(t, err)
	assert.Equal(t, []byte("colon"), v1)

	v2, err := s.Get(ctx, "OD/3345")
	require.NoError(t, err)
	assert.Equal(t, []byte("slash"), v2)
}

func TestNew_FailsWhenDirCannotBeCreated(t *testing.T) {
	// A read-only filesystem can never satisfy MkdirAll.
	fs := afero.NewReadOnlyFs(afero.NewMemMapFs())

	_, err := New(Config{FS: fs, Dir: "/cache/jira"})
	require.Error(t, err)
}

func TestGetWithTTL_UsesInjectedClockNotWallTime(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s, err := New(Config{FS: afero.NewMemMapFs(), Dir: "/cache", Clock: clock})
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, s.Set(ctx, "k1", []byte("v"), store.WithExpiration(time.Hour)))

	// Just before expiry: still a hit.
	clock.now = clock.now.Add(59 * time.Minute)
	value, ttl, err := s.GetWithTTL(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v"), value)
	assert.Positive(t, ttl)

	// Past expiry: a miss, with no wall-clock sleep required to observe it.
	clock.now = clock.now.Add(2 * time.Minute)
	_, err = s.Get(ctx, "k1")
	assert.ErrorIs(t, err, &store.NotFound{})
}

func TestGetType(t *testing.T) {
	s := newTestStore(t)
	assert.Equal(t, Type, s.GetType())
}

func TestInvalidate_IsANoOp(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Invalidate(context.Background()))
}

func TestGetWithTTL_CorruptEntryIsANotFound(t *testing.T) {
	s := newTestStore(t)
	fs := s.fs

	// Write a file too short to even contain the 8-byte expiry header.
	require.NoError(t, afero.WriteFile(fs, s.path("k1"), []byte("x"), fileMode))

	_, _, err := s.GetWithTTL(context.Background(), "k1")
	require.Error(t, err)
	var notFound *store.NotFound
	assert.True(t, errors.As(err, &notFound))
}
