package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
)

func (s *Service2) ensurePersistedSessionLocked() error {
	sessionPath := strings.TrimSpace(s.sessionPath)
	if sessionPath == "" {
		err := sessionPersistenceError{Operation: "verify", Reason: "session path is empty"}
		s.setStateLocked(domain.StateDegradedOrFailed, err.Error())
		return err
	}
	if err := s.ensureSessionFile(sessionPath); err != nil {
		sessionErr := sessionPersistenceError{Operation: "create", Path: sessionPath, Err: err}
		s.setStateLocked(domain.StateDegradedOrFailed, sessionErr.Error())
		return sessionErr
	}
	exists, err := s.fileExists(sessionPath)
	if err != nil {
		sessionErr := sessionPersistenceError{Operation: "verify", Path: sessionPath, Err: err}
		s.setStateLocked(domain.StateDegradedOrFailed, sessionErr.Error())
		return sessionErr
	}
	if !exists {
		sessionErr := sessionPersistenceError{Operation: "verify", Path: sessionPath, Reason: "session file is missing"}
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
		return errors.New("session path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(trimmedPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(trimmedPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}
