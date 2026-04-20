package adapters

import (
	"context"
	"errors"
	"fmt"
	"net"

	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tgerr"
)

const (
	liveErrorKindAuth      liveErrorKind = "auth"
	liveErrorKindNetwork   liveErrorKind = "network"
	liveErrorKindRateLimit liveErrorKind = "rate-limit"
)

var errLiveSessionUnauthorized = errors.New("telegram session is not authorized")

type (
	liveErrorKind   string
	liveSourceError struct {
		err               error
		operation, advice string
		kind              liveErrorKind
	}
)

func (e *liveSourceError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"telegram live %s error during %s: %v; %s",
		e.kind,
		e.operation,
		e.err,
		e.advice,
	)
}

func (e *liveSourceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if typed, ok := errors.AsType[*liveSourceError](err); ok {
		return typed.kind == liveErrorKindAuth
	}
	if errors.Is(err, errLiveSessionUnauthorized) {
		return true
	}
	return tgauth.IsUnauthorized(err) || tgerr.IsCode(err, 401)
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if typed, ok := errors.AsType[*liveSourceError](err); ok {
		return typed.kind == liveErrorKindNetwork
	}
	if isAuthError(err) || isRateLimitError(err) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	_, ok := errors.AsType[*net.OpError](err)
	return ok
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if typed, ok := errors.AsType[*liveSourceError](err); ok {
		return typed.kind == liveErrorKindRateLimit
	}
	_, ok := tgerr.AsFloodWait(err)
	if ok {
		return true
	}
	return tgerr.IsCode(err, 420)
}

func wrapLiveError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*liveSourceError](err); ok {
		return err
	}
	kind := classifyLiveErrorKind(err)
	if kind == "" {
		return fmt.Errorf("telegram live error during %s: %w", operation, err)
	}
	return &liveSourceError{
		kind:      kind,
		operation: operation,
		err:       err,
		advice:    adviceForLiveErrorKind(kind),
	}
}

func classifyLiveErrorKind(err error) liveErrorKind {
	switch {
	case isRateLimitError(err):
		return liveErrorKindRateLimit
	case isAuthError(err):
		return liveErrorKindAuth
	case isNetworkError(err):
		return liveErrorKindNetwork
	default:
		return ""
	}
}

func adviceForLiveErrorKind(kind liveErrorKind) string {
	switch kind {
	case liveErrorKindAuth:
		return "verify TELEGRAM_API_ID/TELEGRAM_API_HASH and ensure a valid authorized session exists at TELEGRAM_SESSION_PATH"
	case liveErrorKindRateLimit:
		return "telegram rate-limited the request (FLOOD_WAIT); wait and retry"
	default:
		return "check local network/proxy connectivity and retry"
	}
}
