package onboarding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/flexer2006/tco/internal/domain"
)

const (
	runtimeProfileReal = "real"

	defaultAuthOperationTimeout = 90 * time.Second

	adviceAuthBackendNotConfigured = "telegram auth backend is not configured"
	sessionOperationVerify         = "verify"
)

type Onboarding struct {
	snapshot                       domain.Snapshot
	sessionPath                    string
	expectedAPIID, expectedAPIHash string
	authBackend                    AuthBackend
	operationTimeout               time.Duration
	now                            func() time.Time
	fileExists                     func(path string) (bool, error)
	ensureSessionFile, removeFile  func(path string) error
	mu                             sync.Mutex
}

func NewOnboarding(sessionPath string, options ...ServiceOption) (*Onboarding, error) {
	service := new(Onboarding{
		sessionPath:       strings.TrimSpace(sessionPath),
		operationTimeout:  defaultAuthOperationTimeout,
		now:               time.Now,
		fileExists:        defaultFileExists,
		ensureSessionFile: defaultEnsureSessionFile,
		removeFile:        os.Remove,
	})

	for _, option := range options {
		if option == nil {
			continue
		}

		err := option(service)
		if err != nil {
			return nil, err
		}
	}

	if service.operationTimeout <= 0 {
		return nil, InvalidInputError{Field: "auth_timeout", Reason: "must be greater than zero"}
	}

	if service.authBackend == nil {
		return nil, InvalidInputError{
			Field:  "auth_backend",
			Reason: "must not be nil when runtime_profile=real",
		}
	}

	service.snapshot = domain.Snapshot{
		RuntimeProfile: runtimeProfileReal,
		SessionPath:    service.sessionPath,
		UpdatedAt:      service.now().UTC(),
	}
	service.probeInitialAuthState()

	return service, nil
}

func (s *Onboarding) Snapshot() domain.Snapshot {
	if s == nil {
		return domain.Snapshot{
			State:  domain.StateDegradedOrFailed,
			Reason: "onboarding service is nil",
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.snapshot
}

func (s *Onboarding) Readiness() domain.Readiness {
	snapshot := s.Snapshot()
	switch snapshot.State {
	case domain.StateReady:
		return domain.Readiness{Ready: true, State: snapshot.State}
	case domain.StateAwaitingCredentials:
		return domain.Readiness{
			Ready:  false,
			State:  snapshot.State,
			Reason: "telegram credentials are required",
		}
	case domain.StateAuthCodeRequested:
		return domain.Readiness{
			Ready:  false,
			State:  snapshot.State,
			Reason: "telegram verification code is required",
		}
	case domain.StateAwaiting2FA:
		return domain.Readiness{
			Ready:  false,
			State:  snapshot.State,
			Reason: "telegram cloud password is required",
		}
	default:
		reason := strings.TrimSpace(snapshot.Reason)
		if reason == "" {
			reason = "onboarding is degraded"
		}

		return domain.Readiness{Ready: false, State: domain.StateDegradedOrFailed, Reason: reason}
	}
}

func (s *Onboarding) Start(ctx context.Context, apiID, apiHash, phone string) error {
	err := ensureContext(ctx)
	if err != nil {
		return err
	}

	apiID = strings.TrimSpace(apiID)

	err = ensureNonEmpty("api_id", apiID)
	if err != nil {
		return err
	}

	apiHash = strings.TrimSpace(apiHash)

	err = ensureNonEmpty("api_hash", apiHash)
	if err != nil {
		return err
	}

	if s.expectedAPIID != "" && apiID != s.expectedAPIID {
		return InvalidInputError{
			Field:  "api_id",
			Reason: "must match TELEGRAM_API_ID from environment",
		}
	}

	if s.expectedAPIHash != "" && apiHash != s.expectedAPIHash {
		return InvalidInputError{
			Field:  "api_hash",
			Reason: "must match TELEGRAM_API_HASH from environment",
		}
	}

	phone = strings.TrimSpace(phone)

	err = ensureNonEmpty("phone", phone)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err = ensureTransition(
		"start",
		s.snapshot.State,
		domain.StateAwaitingCredentials,
		domain.StateDegradedOrFailed,
	)
	if err != nil {
		return err
	}

	if s.authBackend == nil {
		return new(OperationError{
			Kind:      ErrorKindInternal,
			Operation: "start",
			Advice:    adviceAuthBackendNotConfigured,
			Err:       ErrAuthBackendNil,
		})
	}

	err = s.invokeAuthBackendLocked(ctx, "start", func(operationCtx context.Context) error {
		return s.authBackend.Start(operationCtx, apiID, apiHash, phone)
	})
	if err != nil {
		return err
	}

	s.snapshot.Phone = phone
	s.setStateLocked(domain.StateAuthCodeRequested, "")

	return nil
}

func (s *Onboarding) VerifyCode(ctx context.Context, code string) error {
	err := ensureContext(ctx)
	if err != nil {
		return err
	}

	code = strings.TrimSpace(code)

	err = ensureNonEmpty("code", code)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err = ensureTransition(
		"verify_code",
		s.snapshot.State,
		domain.StateAuthCodeRequested,
	)
	if err != nil {
		return err
	}

	if s.authBackend == nil {
		return new(OperationError{
			Kind:      ErrorKindInternal,
			Operation: "verify_code",
			Advice:    adviceAuthBackendNotConfigured,
			Err:       ErrAuthBackendNil,
		})
	}

	err = s.invokeAuthBackendLocked(ctx, "verify_code", func(operationCtx context.Context) error {
		return s.authBackend.VerifyCode(operationCtx, code)
	})
	if err != nil {
		if errors.Is(err, ErrPasswordRequired) {
			s.setStateLocked(domain.StateAwaiting2FA, "")

			return nil
		}

		return err
	}

	err = s.ensurePersistedSessionLocked()
	if err != nil {
		return err
	}

	s.setStateLocked(domain.StateReady, "")

	return nil
}

func (s *Onboarding) VerifyPassword(ctx context.Context, password string) error {
	err := ensureContext(ctx)
	if err != nil {
		return err
	}

	err = ensureNonEmpty("password", password)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err = ensureTransition("verify_password", s.snapshot.State, domain.StateAwaiting2FA)
	if err != nil {
		return err
	}

	if s.authBackend == nil {
		return new(OperationError{
			Kind:      ErrorKindInternal,
			Operation: "verify_password",
			Advice:    adviceAuthBackendNotConfigured,
			Err:       ErrAuthBackendNil,
		})
	}

	err = s.invokeAuthBackendLocked(
		ctx,
		"verify_password",
		func(operationCtx context.Context) error {
			return s.authBackend.VerifyPassword(operationCtx, password)
		},
	)
	if err != nil {
		return err
	}

	err = s.ensurePersistedSessionLocked()
	if err != nil {
		return err
	}

	s.setStateLocked(domain.StateReady, "")

	return nil
}

func (s *Onboarding) Logout(ctx context.Context) error {
	err := ensureContext(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	sessionPath := s.sessionPath
	backend := s.authBackend
	s.snapshot.Phone = ""
	s.mu.Unlock()

	if backend != nil {
		err := backend.Logout(ctx)
		if err != nil {
			s.mu.Lock()
			s.setStateLocked(
				domain.StateDegradedOrFailed,
				fmt.Sprintf("telegram logout failed: %v", err),
			)
			s.mu.Unlock()

			return err
		}
	}

	if sessionPath != "" {
		err := s.removeFile(sessionPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.mu.Lock()
			s.setStateLocked(
				domain.StateDegradedOrFailed,
				fmt.Sprintf("remove session file: %v", err),
			)
			s.mu.Unlock()

			return fmt.Errorf("remove session file: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.setStateLocked(domain.StateAwaitingCredentials, "")

	return nil
}

func (s *Onboarding) probeInitialAuthState() {
	if s.sessionPath == "" {
		s.setStateLocked(domain.StateAwaitingCredentials, "")

		return
	}

	exists, err := s.fileExists(s.sessionPath)
	if err != nil {
		s.setStateLocked(
			domain.StateDegradedOrFailed,
			fmt.Sprintf("session check failed: %v", err),
		)

		return
	}

	if !exists {
		s.setStateLocked(domain.StateAwaitingCredentials, "")

		return
	}

	authCtx, authCancel := context.WithTimeout(context.Background(), s.operationTimeout)
	authorized, authErr := s.authBackend.Authorized(authCtx)

	authCancel()

	if authErr != nil {
		s.setStateLocked(domain.StateDegradedOrFailed, "session authorization check failed")

		return
	}

	if !authorized {
		s.setStateLocked(
			domain.StateAwaitingCredentials,
			"telegram session file exists but is not authorized",
		)

		return
	}

	s.setStateLocked(domain.StateReady, "")
}

func (s *Onboarding) setStateLocked(state domain.State, reason string) {
	s.snapshot.State = state
	s.snapshot.Reason = strings.TrimSpace(reason)
	s.snapshot.UpdatedAt = s.now().UTC()
}

func (s *Onboarding) invokeAuthBackendLocked(
	ctx context.Context,
	operation string,
	callback func(ctx context.Context) error,
) error {
	if callback == nil {
		return new(OperationError{
			Kind:      ErrorKindInternal,
			Operation: operation,
			Advice:    "telegram auth callback is not configured",
		})
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

			return new(copied)
		}

		return operationErr
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
		return new(OperationError{
			Kind:      ErrorKindTimeout,
			Operation: operation,
			Advice:    "telegram auth operation timed out; retry the request",
			Err:       err,
		})
	}

	return new(OperationError{
		Kind:      ErrorKindInternal,
		Operation: operation,
		Advice:    "telegram auth operation failed; retry or inspect service logs",
		Err:       err,
	})
}
