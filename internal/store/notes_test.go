package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Notes(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	p := "/Users/test/notesproj"

	list, err := s.ListNotes(p)
	require.NoError(t, err)
	assert.Empty(t, list)

	require.NoError(t, s.SetNote(p, "staging-db", "listens on port 5433"))
	require.NoError(t, s.SetNote(p, "deploy", "ops/deploy.sh"))

	list, err = s.ListNotes(p)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	n, err := s.GetNote(p, "staging-db")
	require.NoError(t, err)
	assert.Equal(t, "listens on port 5433", n.Body)
	assert.False(t, n.CreatedAt.IsZero())

	// update overwrites body, keeps the entry
	require.NoError(t, s.SetNote(p, "staging-db", "moved to 5444"))
	n, err = s.GetNote(p, "staging-db")
	require.NoError(t, err)
	assert.Equal(t, "moved to 5444", n.Body)

	require.NoError(t, s.DeleteNote(p, "deploy"))
	list, err = s.ListNotes(p)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// invalid alias is rejected
	assert.Error(t, s.SetNote(p, "bad alias!", "x"))

	// notes never leak into the credential store
	creds, err := s.ListCredentials(p)
	require.NoError(t, err)
	assert.Empty(t, creds)
}
