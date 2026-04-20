package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

const (
	runtimeProfileReal = "real"

	defaultAuthOperationTimeout = 90 * time.Second
)

type Service2 struct {
	snapshot                      domain.Snapshot
	sessionPath                   string
	authBackend                   AuthBackend
	operationTimeout              time.Duration
	now                           func() time.Time
	fileExists                    func(path string) (bool, error)
	ensureSessionFile, removeFile func(path string) error
	mu                            sync.Mutex
}

func NewService2(sessionPath string, options ...ServiceOption) (*Service2, error) {
	service := &Service2{
		sessionPath:       strings.TrimSpace(sessionPath),
		operationTimeout:  defaultAuthOperationTimeout,
		now:               time.Now,
		fileExists:        defaultFileExists,
		ensureSessionFile: defaultEnsureSessionFile,
		removeFile:        os.Remove,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	if service.operationTimeout <= 0 {
		return nil, InvalidInputError{Field: "auth_timeout", Reason: "must be greater than zero"}
	}
	if service.authBackend == nil {
		return nil, InvalidInputError{Field: "auth_backend", Reason: "must not be nil when runtime_profile=real"}
	}
	service.snapshot = domain.Snapshot{
		RuntimeProfile: runtimeProfileReal,
		SessionPath:    service.sessionPath,
		UpdatedAt:      service.now().UTC(),
	}
	if service.sessionPath == "" {
		service.setStateLocked(domain.StateAwaitingCredentials, "")
		return service, nil
	}
	exists, err := service.fileExists(service.sessionPath)
	if err != nil {
		service.setStateLocked(domain.StateDegradedOrFailed, fmt.Sprintf("session check failed: %v", err))
		return service, nil
	}
	if exists {
		service.setStateLocked(domain.StateReady, "")
		return service, nil
	}
	service.setStateLocked(domain.StateAwaitingCredentials, "")
	return service, nil
}

func (s *Service2) Snapshot() domain.Snapshot {
	if s == nil {
		return domain.Snapshot{State: domain.StateDegradedOrFailed, Reason: "onboarding service is nil"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *Service2) Readiness() domain.Readiness {
	snapshot := s.Snapshot()
	switch snapshot.State {
	case domain.StateReady:
		return domain.Readiness{Ready: true, State: snapshot.State}
	case domain.StateAwaitingCredentials:
		return domain.Readiness{Ready: false, State: snapshot.State, Reason: "telegram credentials are required"}
	case domain.StateAuthCodeRequested:
		return domain.Readiness{Ready: false, State: snapshot.State, Reason: "telegram verification code is required"}
	case domain.StateAwaiting2FA:
		return domain.Readiness{Ready: false, State: snapshot.State, Reason: "telegram cloud password is required"}
	default:
		reason := strings.TrimSpace(snapshot.Reason)
		if reason == "" {
			reason = "onboarding is degraded"
		}
		return domain.Readiness{Ready: false, State: domain.StateDegradedOrFailed, Reason: reason}
	}
}

func (s *Service2) Start(ctx context.Context, apiID, apiHash, phone string) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	apiID = strings.TrimSpace(apiID)
	if err := ensureNonEmpty("api_id", apiID); err != nil {
		return err
	}
	apiHash = strings.TrimSpace(apiHash)
	if err := ensureNonEmpty("api_hash", apiHash); err != nil {
		return err
	}
	phone = strings.TrimSpace(phone)
	if err := ensureNonEmpty("phone", phone); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensureTransition("start", s.snapshot.State, domain.StateAwaitingCredentials, domain.StateDegradedOrFailed); err != nil {
		return err
	}
	if s.authBackend == nil {
		return &OperationError{
			Kind:      ErrorKindInternal,
			Operation: "start",
			Advice:    "telegram auth backend is not configured",
			Err:       errors.New("telegram auth backend is nil"),
		}
	}
	if err := s.invokeAuthBackendLocked(ctx, "start", func(operationCtx context.Context) error {
		return s.authBackend.Start(operationCtx, apiID, apiHash, phone)
	}); err != nil {
		return err
	}
	s.snapshot.Phone = phone
	s.setStateLocked(domain.StateAuthCodeRequested, "")
	return nil
}

func (s *Service2) VerifyCode(ctx context.Context, code string) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if err := ensureNonEmpty("code", code); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensureTransition("verify_code", s.snapshot.State, domain.StateAuthCodeRequested); err != nil {
		return err
	}
	if s.authBackend == nil {
		return &OperationError{
			Kind:      ErrorKindInternal,
			Operation: "verify_code",
			Advice:    "telegram auth backend is not configured",
			Err:       errors.New("telegram auth backend is nil"),
		}
	}
	err := s.invokeAuthBackendLocked(ctx, "verify_code", func(operationCtx context.Context) error {
		return s.authBackend.VerifyCode(operationCtx, code)
	})
	if err != nil {
		if errors.Is(err, ErrPasswordRequired) {
			s.setStateLocked(domain.StateAwaiting2FA, "")
			return nil
		}
		return err
	}
	if err := s.ensurePersistedSessionLocked(); err != nil {
		return err
	}
	s.setStateLocked(domain.StateReady, "")
	return nil
}

func (s *Service2) VerifyPassword(ctx context.Context, password string) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	if err := ensureNonEmpty("password", password); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ensureTransition("verify_password", s.snapshot.State, domain.StateAwaiting2FA); err != nil {
		return err
	}
	if s.authBackend == nil {
		return &OperationError{
			Kind:      ErrorKindInternal,
			Operation: "verify_password",
			Advice:    "telegram auth backend is not configured",
			Err:       errors.New("telegram auth backend is nil"),
		}
	}
	if err := s.invokeAuthBackendLocked(ctx, "verify_password", func(operationCtx context.Context) error {
		return s.authBackend.VerifyPassword(operationCtx, password)
	}); err != nil {
		return err
	}
	if err := s.ensurePersistedSessionLocked(); err != nil {
		return err
	}
	s.setStateLocked(domain.StateReady, "")
	return nil
}

func (s *Service2) Logout(ctx context.Context) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	sessionPath := s.sessionPath
	s.snapshot.Phone = ""
	s.mu.Unlock()
	if sessionPath != "" {
		if err := s.removeFile(sessionPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.mu.Lock()
			s.setStateLocked(domain.StateDegradedOrFailed, fmt.Sprintf("remove session file: %v", err))
			s.mu.Unlock()
			return fmt.Errorf("remove session file: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStateLocked(domain.StateAwaitingCredentials, "")
	return nil
}

func (s *Service2) setStateLocked(state domain.State, reason string) {
	s.snapshot.State = state
	s.snapshot.Reason = strings.TrimSpace(reason)
	s.snapshot.UpdatedAt = s.now().UTC()
}

func (s *Service2) invokeAuthBackendLocked(ctx context.Context, operation string, callback func(ctx context.Context) error) error {
	if callback == nil {
		return &OperationError{
			Kind:      ErrorKindInternal,
			Operation: operation,
			Advice:    "telegram auth callback is not configured",
		}
	}
	operationCtx, cancel := context.WithTimeout(ctx, s.operationTimeout)
	defer cancel()
	err := callback(operationCtx)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPasswordRequired) {
		return err
	}
	if operationErr, ok := errors.AsType[*OperationError](err); ok {
		if strings.TrimSpace(operationErr.Operation) == "" {
			copied := *operationErr
			copied.Operation = operation
			return &copied
		}
		return operationErr
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
		return &OperationError{
			Kind:      ErrorKindTimeout,
			Operation: operation,
			Advice:    "telegram auth operation timed out; retry the request",
			Err:       err,
		}
	}
	return &OperationError{
		Kind:      ErrorKindInternal,
		Operation: operation,
		Advice:    "telegram auth operation failed; retry or inspect service logs",
		Err:       err,
	}
}
