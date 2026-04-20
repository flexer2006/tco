package application

import (
	"errors"
	"fmt"
	"strings"
	"github.com/flexer2006/tco/internal/domain"
	"time"
)

const (
	ErrorKindAuth          ErrorKind = "auth_error"
	ErrorKindNetwork       ErrorKind = "network_error"
	ErrorKindRateLimit     ErrorKind = "rate_limit_error"
	ErrorKindInvalidTarget ErrorKind = "invalid_target_error"
	ErrorKindTimeout       ErrorKind = "timeout_error"
	ErrorKindInternal      ErrorKind = "internal_error"
)

var (
	ErrPasswordRequired   = errors.New("onboarding password required")
	ErrInvalidInput       = errors.New("onboarding invalid input")
	ErrInvalidTransition  = errors.New("onboarding invalid transition")
	errSessionPersistence = errors.New("onboarding session persistence failed")
)

type (
	ErrorKind      string
	OperationError struct {
		Kind              ErrorKind
		Operation, Advice string
		Err               error
		RetryAfter        time.Duration
	}
	InvalidInputError struct {
		Field, Reason string
	}
	invalidTransitionError struct {
		Allowed []domain.State
		Action  string
		From    domain.State
	}
	sessionPersistenceError struct {
		Operation, Path, Reason string
		Err                     error
	}
)

func (e *OperationError) Error() string {
	if e == nil {
		return ""
	}
	kind := e.KindOrDefault()
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "operation"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s failed (%s): %v", operation, kind, e.Err)
	}
	advice := strings.TrimSpace(e.Advice)
	if advice == "" {
		return fmt.Sprintf("%s failed (%s)", operation, kind)
	}
	return fmt.Sprintf("%s failed (%s): %s", operation, kind, advice)
}

func (e *OperationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *OperationError) KindOrDefault() ErrorKind {
	if e == nil {
		return ErrorKindInternal
	}
	if strings.TrimSpace(string(e.Kind)) == "" {
		return ErrorKindInternal
	}
	return e.Kind
}

func (e InvalidInputError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Sprintf("%s: invalid value", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func (e InvalidInputError) Is(target error) bool {
	return target == ErrInvalidInput
}

func (e invalidTransitionError) Error() string {
	allowed := make([]string, 0, len(e.Allowed))
	for _, state := range e.Allowed {
		allowed = append(allowed, string(state))
	}
	return fmt.Sprintf("%s: transition from %q is not allowed (allowed: %s)", e.Action, e.From, strings.Join(allowed, ", "))
}

func (e invalidTransitionError) Is(target error) bool {
	return target == ErrInvalidTransition
}

func (e sessionPersistenceError) Error() string {
	path := strings.TrimSpace(e.Path)
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "verify"
	}
	if e.Err != nil {
		if path == "" {
			return fmt.Sprintf("session persistence %s failed: %v", operation, e.Err)
		}
		return fmt.Sprintf("session persistence %s failed for %q: %v", operation, path, e.Err)
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "unknown reason"
	}
	if path == "" {
		return fmt.Sprintf("session persistence %s failed: %s", operation, reason)
	}
	return fmt.Sprintf("session persistence %s failed for %q: %s", operation, path, reason)
}

func (e sessionPersistenceError) Unwrap() error {
	return e.Err
}

func (e sessionPersistenceError) Is(target error) bool {
	return target == errSessionPersistence
}
