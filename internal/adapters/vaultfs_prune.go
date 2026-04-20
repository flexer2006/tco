package adapters

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//nolint:gocyclo
func pruneManagedFiles(root string, desired map[string]struct{}) (int, error) {
	pruned := 0
	managedRoots := []string{
		filepath.Join(root, "topics"),
		filepath.Join(root, "_meta", "embeddings"),
	}
	for _, managedRoot := range managedRoots {
		if _, err := os.Stat(managedRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
		var dirs []string
		if err := filepath.WalkDir(managedRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != managedRoot {
					dirs = append(dirs, path)
				}
				return nil
			}
			if _, ok := desired[path]; ok {
				return nil
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			pruned++
			return nil
		}); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return 0, err
		}
		slices.SortFunc(dirs, func(a, b string) int {
			if n := len(b) - len(a); n != 0 {
				return n
			}
			return strings.Compare(a, b)
		})
		for _, dir := range dirs {
			_ = os.Remove(dir)
		}
	}
	return pruned, nil
}
