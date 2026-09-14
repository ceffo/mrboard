package gitlabadpt

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndraft_PassesThrough(t *testing.T) {
	c := &fakeDescriptionClient{}
	a := &GitLabAdapter{client: c}

	err := a.Undraft(context.Background(), 1, 10)
	require.NoError(t, err)

	assert.Equal(t, 1, c.undraftCalls, "expected Undraft called once")
}

func TestUndraft_PropagatesError(t *testing.T) {
	boom := errors.New("write error")
	c := &fakeDescriptionClient{undraftErr: boom}
	a := &GitLabAdapter{client: c}

	err := a.Undraft(context.Background(), 1, 10)
	assert.ErrorIs(t, err, boom, "expected wrapped write error, got %v", err)
}
