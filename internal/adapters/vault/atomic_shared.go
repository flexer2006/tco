package vault

import (
	"bytes"
	"errors"
	"fmt"
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
		writeTemp: writeTempFileSync,
		rename:    os.Rename,
		remove:    os.Remove,
	}
}

func writeTempFileSync(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, fileModePrivate)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("write temp file %q: %w", path, err)
	}

	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync temp file %q: %w", path, err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close temp file %q: %w", path, err)
	}

	return nil
}

func (w sharedAtomicWriter) write(path string, data []byte) (bool, error) {
	existing, err := w.readFile(path)
	switch {
	case err == nil && bytes.Equal(existing, data):
		return false, nil
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return false, err
	}

	err = w.mkdirAll(filepath.Dir(path), dirModePublic)
	if err != nil {
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

	err = w.writeTemp(tempPath, data)
	if err != nil {
		return false, err
	}

	err = w.rename(tempPath, path)
	if err != nil {
		return false, err
	}

	cleanup = false

	return true, nil
}
