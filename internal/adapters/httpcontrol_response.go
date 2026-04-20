package adapters

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

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
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
		writeJSON(w, http.StatusForbidden, map[string]any{"status": "forbidden", "error": "invalid csrf token"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"status": "invalid_input", "error": "invalid form payload"})
}

func writeOnboardingStatus(w http.ResponseWriter, onboarding OnboardingService, err error) {
	snapshot := onboarding.Snapshot()
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"auth":   snapshot,
		})
		return
	}
	statusCode, status := http.StatusInternalServerError, "error"
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		statusCode = http.StatusBadRequest
		status = "invalid_input"
	case errors.Is(err, application.ErrInvalidTransition):
		statusCode = http.StatusConflict
		status = "invalid_transition"
	}
	payload := map[string]any{
		"status": status,
		"error":  err.Error(),
		"auth":   snapshot,
	}
	if operationErr, ok := errors.AsType[*application.OperationError](err); ok {
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
		switch kind {
		case application.ErrorKindAuth:
			statusCode = http.StatusUnauthorized
		case application.ErrorKindNetwork:
			statusCode = http.StatusServiceUnavailable
		case application.ErrorKindRateLimit:
			statusCode = http.StatusTooManyRequests
		case application.ErrorKindInvalidTarget:
			statusCode = http.StatusBadRequest
		case application.ErrorKindTimeout:
			statusCode = http.StatusGatewayTimeout
		case application.ErrorKindInternal:
			statusCode = http.StatusInternalServerError
		default:
			statusCode = http.StatusInternalServerError
		}
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
	}
	writeJSON(w, statusCode, payload)
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
