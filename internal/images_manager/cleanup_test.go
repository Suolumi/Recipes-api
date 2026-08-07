package images_manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCleanupUnreferenced(t *testing.T) {
	directory := t.TempDir()
	kept := primitive.NewObjectID().Hex() + ".jpg"
	removed := primitive.NewObjectID().Hex() + ".png"
	newPicture := primitive.NewObjectID().Hex() + ".jpg"
	legacy := "legacy-picture.jpg"
	for _, name := range []string{kept, removed, newPicture, legacy} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("image"), 0o600))
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{kept, removed, legacy} {
		require.NoError(t, os.Chtimes(filepath.Join(directory, name), old, old))
	}

	removedCount, err := CleanupUnreferenced(directory, map[string]struct{}{kept: {}}, time.Now(), 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, removedCount)
	_, err = os.Stat(filepath.Join(directory, removed))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, name := range []string{kept, newPicture, legacy} {
		_, err := os.Stat(filepath.Join(directory, name))
		require.NoError(t, err)
	}
}
