package httpcontrol

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/flexer2006/tco/internal/application"
)

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set(
		"Content-Security-Policy",
		"default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'",
	)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	setSecurityHeaders(w)

	raw, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\"status\":\"error\",\"error\":\"json_encode_failed\"}\n"))

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(append(raw, '\n'))
}

func writeAuthPostValidationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	if errors.Is(err, errInvalidCSRFToken) {
		writeJSON(
			w,
			http.StatusForbidden,
			map[string]any{jsonKeyStatus: statusForbidden, jsonKeyError: "invalid csrf token"},
		)

		return
	}

	writeJSON(
		w,
		http.StatusBadRequest,
		map[string]any{jsonKeyStatus: statusInvalidInput, jsonKeyError: "invalid form payload"},
	)
}

func writeOnboardingStatus(w http.ResponseWriter, onboarding OnboardingService, err error) {
	snapshot := onboarding.Snapshot()
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			jsonKeyStatus: statusOK,
			"auth":        publicAuthSnapshot(snapshot),
		})

		return
	}

	statusCode, status := http.StatusInternalServerError, statusError

	switch {
	case errors.Is(err, application.ErrInvalidInput):
		statusCode = http.StatusBadRequest
		status = statusInvalidInput
	case errors.Is(err, application.ErrInvalidTransition):
		statusCode = http.StatusConflict
		status = "invalid_transition"
	}

	payload := map[string]any{
		jsonKeyStatus: status,
		jsonKeyError:  publicOnboardingError(err),
		"auth":        publicAuthSnapshot(snapshot),
	}
	statusCode = applyOperationErrorDetails(w, payload, err, statusCode)

	writeJSON(w, statusCode, payload)
}

func applyOperationErrorDetails(
	w http.ResponseWriter,
	payload map[string]any,
	err error,
	statusCode int,
) int {
	operationErr, ok := errors.AsType[*application.OperationError](err)
	if !ok {
		return statusCode
	}

	kind := operationErr.KindOrDefault()

	operation := strings.TrimSpace(operationErr.Operation)
	if operation == "" {
		operation = "unknown"
	}

	remediationHint := remediationHintForOperationError(operationErr)
	slog.Warn("onboarding operation failed",
		"error_class", string(kind),
		"remediation_hint", remediationHint,
		"operation", operation,
	)

	payload["error_kind"] = kind
	statusCode = statusCodeForErrorKind(kind)

	if operationErr.RetryAfter > 0 {
		retryAfterSeconds := int(math.Ceil(operationErr.RetryAfter.Seconds()))

		payload["retry_after_seconds"] = retryAfterSeconds
		if kind == application.ErrorKindRateLimit {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
		}
	}

	if remediationHint != "" {
		payload["error_advice"] = remediationHint
	}

	return statusCode
}

func statusCodeForErrorKind(kind application.ErrorKind) int {
	switch kind {
	case application.ErrorKindAuth:
		return http.StatusUnauthorized
	case application.ErrorKindNetwork:
		return http.StatusServiceUnavailable
	case application.ErrorKindRateLimit:
		return http.StatusTooManyRequests
	case application.ErrorKindInvalidTarget:
		return http.StatusBadRequest
	case application.ErrorKindTimeout:
		return http.StatusGatewayTimeout
	case application.ErrorKindInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

func publicOnboardingError(err error) string {
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		return err.Error()
	case errors.Is(err, application.ErrInvalidTransition):
		return err.Error()
	default:
		if operationErr, ok := errors.AsType[*application.OperationError](err); ok {
			if advice := strings.TrimSpace(operationErr.Advice); advice != "" {
				return advice
			}
		}

		return "onboarding operation failed"
	}
}

func remediationHintForOperationError(operationErr *application.OperationError) string {
	if operationErr == nil {
		return ""
	}

	if advice := strings.TrimSpace(operationErr.Advice); advice != "" {
		return advice
	}

	switch operationErr.KindOrDefault() {
	default:
		return "retry the request or inspect service logs"
	case application.ErrorKindAuth:
		return "verify credentials and retry"
	case application.ErrorKindNetwork:
		return "check network connectivity and retry"
	case application.ErrorKindRateLimit:
		return "wait for retry window before retrying"
	case application.ErrorKindInvalidTarget:
		return "check target format and access rights"
	case application.ErrorKindTimeout:
		return "retry the request and verify Telegram connectivity"
	}
}
