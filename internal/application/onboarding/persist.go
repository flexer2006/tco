package onboarding

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flexer2006/tco/internal/domain"
)

const (
	sessionDirMode  = 0o700
	sessionFileMode = 0o600
)

func (s *Onboarding) ensurePersistedSessionLocked() error {
	sessionPath := strings.TrimSpace(s.sessionPath)
	if sessionPath == "" {
		err := sessionPersistenceError{Operation: sessionOperationVerify, Reason: "session path is empty"}
		s.setStateLocked(domain.StateDegradedOrFailed, err.Error())

		return err
	}

	err := s.ensureSessionFile(sessionPath)
	if err != nil {
		sessionErr := sessionPersistenceError{Operation: "create", Path: sessionPath, Err: err}
		s.setStateLocked(domain.StateDegradedOrFailed, sessionErr.Error())

		return sessionErr
	}

	exists, err := s.fileExists(sessionPath)
	if err != nil {
		sessionErr := sessionPersistenceError{Operation: sessionOperationVerify, Path: sessionPath, Err: err}
		s.setStateLocked(domain.StateDegradedOrFailed, sessionErr.Error())

		return sessionErr
	}

	if !exists {
		sessionErr := sessionPersistenceError{
			Operation: sessionOperationVerify,
			Path:      sessionPath,
			Reason:    "session file is missing",
		}
		s.setStateLocked(domain.StateDegradedOrFailed, sessionErr.Error())

		return sessionErr
	}

	return nil
}

func defaultFileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, err
}

func defaultEnsureSessionFile(path string) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ErrSessionPathEmpty
	}

	err := os.MkdirAll(filepath.Dir(trimmedPath), sessionDirMode)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(trimmedPath, os.O_RDWR|os.O_CREATE, sessionFileMode)
	if err != nil {
		return err
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close session file %q: %w", trimmedPath, err)
	}

	return nil
}
