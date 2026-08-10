package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexer2006/tco/internal/ports"
)

type fileProjectionDirtyMarker struct {
	path string
}

func NewProjectionDirtyMarker(vaultRoot string) (ports.ProjectionDirtyMarker, error) {
	trimmed := strings.TrimSpace(vaultRoot)
	if trimmed == "" {
		return nil, ErrVaultRootEmpty
	}

	return fileProjectionDirtyMarker{
		path: filepath.Join(filepath.Clean(trimmed), "_meta", "projection.dirty"),
	}, nil
}

func (m fileProjectionDirtyMarker) IsDirty() (bool, error) {
	_, err := os.Stat(m.path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat projection dirty marker: %w", err)
}

func (m fileProjectionDirtyMarker) MarkDirty() error {
	err := os.MkdirAll(filepath.Dir(m.path), dirModePublic)
	if err != nil {
		return fmt.Errorf("create projection dirty marker dir: %w", err)
	}

	err = os.WriteFile(m.path, []byte("1\n"), fileModePrivate)
	if err != nil {
		return fmt.Errorf("write projection dirty marker: %w", err)
	}

	return nil
}

func (m fileProjectionDirtyMarker) ClearDirty() error {
	err := os.Remove(m.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear projection dirty marker: %w", err)
	}

	return nil
}
