package images_manager

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func CleanupUnreferenced(directory string, references map[string]struct{}, now time.Time, grace time.Duration) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !isGeneratedPictureName(entry.Name()) {
			continue
		}
		if _, ok := references[entry.Name()]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return removed, err
		}
		if now.Sub(info.ModTime()) < grace {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func isGeneratedPictureName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return false
	}
	_, err := primitive.ObjectIDFromHex(strings.TrimSuffix(name, filepath.Ext(name)))
	return err == nil
}
