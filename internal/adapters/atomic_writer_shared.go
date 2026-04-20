package adapters

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
)

type sharedAtomicWriter struct {
	readFile   func(string) ([]byte, error)
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (string, error)
	writeTemp  func(string, []byte) error
	rename     func(string, string) error
	remove     func(string) error
}

func newSharedAtomicWriter() sharedAtomicWriter {
	return sharedAtomicWriter{
		readFile: os.ReadFile,
		mkdirAll: os.MkdirAll,
		createTemp: func(dir, pattern string) (string, error) {
			file, err := os.CreateTemp(dir, pattern)
			if err != nil {
				return "", err
			}
			name := file.Name()
			_ = file.Close()
			return name, nil
		},
		writeTemp: func(path string, data []byte) error {
			return os.WriteFile(path, data, 0o600)
		},
		rename: os.Rename,
		remove: os.Remove,
	}
}

func (w sharedAtomicWriter) write(path string, data []byte) (bool, error) {
	existing, err := w.readFile(path)
	switch {
	case err == nil && bytes.Equal(existing, data):
		return false, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, err
	}
	if err := w.mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	tempPath, err := w.createTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return false, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = w.remove(tempPath)
		}
	}()
	if err := w.writeTemp(tempPath, data); err != nil {
		return false, err
	}
	if err := w.rename(tempPath, path); err != nil {
		return false, err
	}
	cleanup = false
	return true, nil
}
