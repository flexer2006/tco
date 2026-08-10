package vault

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

func noteMarkdownPath(vaultRoot, clusterSlug, noteID string) (string, error) {
	err := validateStem(clusterSlug)
	if err != nil {
		return "", fmt.Errorf("cluster slug: %w", err)
	}

	err = validateStem(noteID)
	if err != nil {
		return "", fmt.Errorf("note id: %w", err)
	}

	return filepath.Join(vaultRoot, "topics", clusterSlug, noteID+".md"), nil
}

func clusterIndexPath(vaultRoot, clusterSlug string) (string, error) {
	err := validateStem(clusterSlug)
	if err != nil {
		return "", fmt.Errorf("cluster slug: %w", err)
	}

	return filepath.Join(vaultRoot, "topics", clusterSlug, "index.md"), nil
}

func embeddingSidecarPath(vaultRoot, noteID string) (string, error) {
	err := validateStem(noteID)
	if err != nil {
		return "", fmt.Errorf("note id: %w", err)
	}

	return filepath.Join(vaultRoot, "_meta", "embeddings", noteID+".json"), nil
}

func validateStem(stem string) error {
	if strings.TrimSpace(stem) != stem {
		return ErrStemWhitespace
	}

	if stem == "" {
		return ErrStemEmpty
	}

	if stem == "." || stem == ".." {
		return ErrStemPathSegment
	}

	if strings.ContainsAny(stem, `/\\`) {
		return ErrStemPathSeparators
	}

	for _, r := range stem {
		if r == 0 || unicode.IsControl(r) {
			return ErrStemControlChars
		}
	}

	return nil
}
