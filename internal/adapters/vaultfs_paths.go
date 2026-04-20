package adapters

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

func noteMarkdownPath(vaultRoot, clusterSlug, noteID string) (string, error) {
	if err := validateStem(clusterSlug); err != nil {
		return "", fmt.Errorf("cluster slug: %w", err)
	}
	if err := validateStem(noteID); err != nil {
		return "", fmt.Errorf("note id: %w", err)
	}
	return filepath.Join(vaultRoot, "topics", clusterSlug, noteID+".md"), nil
}

func clusterIndexPath(vaultRoot, clusterSlug string) (string, error) {
	if err := validateStem(clusterSlug); err != nil {
		return "", fmt.Errorf("cluster slug: %w", err)
	}
	return filepath.Join(vaultRoot, "topics", clusterSlug, "index.md"), nil
}

func embeddingSidecarPath(vaultRoot, noteID string) (string, error) {
	if err := validateStem(noteID); err != nil {
		return "", fmt.Errorf("note id: %w", err)
	}
	return filepath.Join(vaultRoot, "_meta", "embeddings", noteID+".json"), nil
}

func validateStem(stem string) error {
	if strings.TrimSpace(stem) != stem {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if stem == "" {
		return errors.New("must not be empty")
	}
	if stem == "." || stem == ".." {
		return errors.New("must not be a path segment")
	}
	if strings.ContainsAny(stem, `/\\`) {
		return errors.New("must not contain path separators")
	}
	for _, r := range stem {
		if r == 0 || unicode.IsControl(r) {
			return errors.New("must not contain control characters")
		}
	}
	return nil
}
