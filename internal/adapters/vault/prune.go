package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func pruneManagedFiles(root string, desired map[string]struct{}) (int, error) {
	pruned := 0

	managedRoots := []string{
		filepath.Join(root, "topics"),
		filepath.Join(root, "_meta", "embeddings"),
	}
	for _, managedRoot := range managedRoots {
		fsRoot, err := os.OpenRoot(managedRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return 0, fmt.Errorf("open managed vault root %q: %w", managedRoot, err)
		}

		var dirs []string

		walkErr := fs.WalkDir(
			fsRoot.FS(),
			".",
			func(rel string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}

				if rel == "." {
					return nil
				}

				fullPath := filepath.Join(managedRoot, rel)
				if entry.IsDir() {
					dirs = append(dirs, rel)

					return nil
				}

				if _, ok := desired[fullPath]; ok {
					return nil
				}

				if !isManagedVaultArtifact(root, fullPath) {
					return nil
				}

				err := fsRoot.Remove(rel)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove managed vault artifact %q: %w", fullPath, err)
				}

				pruned++

				return nil
			})
		if walkErr != nil {
			_ = fsRoot.Close()

			return 0, fmt.Errorf("walk managed vault root %q: %w", managedRoot, walkErr)
		}

		slices.SortFunc(dirs, func(a, b string) int {
			if n := len(b) - len(a); n != 0 {
				return n
			}

			return strings.Compare(a, b)
		})

		for _, dir := range dirs {
			_ = fsRoot.Remove(dir) // only removes empty dirs
		}

		err = fsRoot.Close()
		if err != nil {
			return 0, fmt.Errorf("close managed vault root %q: %w", managedRoot, err)
		}
	}

	return pruned, nil
}

func isManagedVaultArtifact(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasPrefix(rel, "_meta/embeddings/") && strings.HasSuffix(rel, ".json"):
		return true
	case strings.HasPrefix(rel, "topics/") && strings.HasSuffix(rel, "/index.md"):
		return true
	case strings.HasPrefix(rel, "topics/") && strings.HasSuffix(rel, ".md"):
		base := filepath.Base(rel)

		return base != "index.md" && strings.Contains(base, ":")
	default:
		return false
	}
}
